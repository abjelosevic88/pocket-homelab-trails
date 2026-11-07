package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// exportDoc is everything of one person, to keep or to bring to another box.
type exportDoc struct {
	App        string          `json:"app"`
	Version    string          `json:"version"`
	ExportedAt string          `json:"exported_at"`
	Routes     []*store.Route  `json:"routes"`
	Outings    []*store.Outing `json:"outings"`
	Gear       []*store.Gear   `json:"gear"`
	Packs      []*store.Pack   `json:"packs"`
}

func (s *Server) export(ctx context.Context, uid int64) (exportDoc, error) {
	doc := exportDoc{App: "pocket-trails", Version: Version, ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, arch := range []bool{false, true} {
		list, err := s.store.ListRoutes(ctx, uid, store.RouteFilter{Archived: arch})
		if err != nil {
			return doc, err
		}
		for _, rt := range list {
			full, err := s.store.GetRoute(ctx, uid, rt.ID)
			if err != nil {
				return doc, err
			}
			full.Preview = nil
			doc.Routes = append(doc.Routes, full)
		}
	}
	outs, _, err := s.store.ListOutings(ctx, uid, store.OutingFilter{})
	if err != nil {
		return doc, err
	}
	for _, o := range outs {
		full, _, err := s.store.GetOuting(ctx, uid, o.ID)
		if err != nil {
			return doc, err
		}
		doc.Outings = append(doc.Outings, full)
	}
	for _, arch := range []bool{false, true} {
		g, err := s.store.ListGear(ctx, uid, arch, "")
		if err != nil {
			return doc, err
		}
		doc.Gear = append(doc.Gear, g...)
	}
	if doc.Packs, err = s.store.ListPacks(ctx, uid); err != nil {
		return doc, err
	}
	for _, p := range doc.Packs {
		for _, it := range p.Items {
			it.Gear = nil
		}
	}
	return doc, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc, err := s.export(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-trails-export.json"`)
	writeJSON(w, http.StatusOK, doc)
}

// handleExportCSV is every outing in one table (a spreadsheet opens it).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	outs, facts, err := s.store.ListOutings(ctx, uid, store.OutingFilter{Desc: true})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-trails-outings.csv"`)
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Date", "Start", "Outing", "Activity", "Route", "Status", "Distance (km)", "Climb (m)", "Time (min)", "With", "Rating", "Trail conditions", "Notes"})
	for i, o := range outs {
		decorateOuting(o, facts[i], st.PacePct)
		d, g, m := walked(o)
		_ = cw.Write([]string{o.Date, o.StartTime, outingTitle(o), o.Activity, o.RouteName, o.Status, fmt.Sprintf("%.2f", d/1000), fmt.Sprintf("%.0f", g), fmt.Sprint(m),
			peopleNames(o.People), fmt.Sprint(o.Rating), o.Conditions, firstNonEmpty(o.Report, o.Notes)})
	}
	cw.Flush()
}

// handleImport takes a Pocket Trails export (again and again: nothing is
// doubled). Route files (GPX…) go through /routes/import.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		if data, err = io.ReadAll(f); err != nil {
			writeErr(w, http.StatusBadRequest, "could not read the file")
			return
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		data = b
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	var doc exportDoc
	if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-trails" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Trails export (for GPX, KML or TCX files use Routes → Add → From a file)")
		return
	}
	res, err := s.importPocket(ctx, uid, doc)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// importPocket brings back an export. Each thing remembers where it came
// from (source + ref), so a second import finds it instead of doubling it.
func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) (map[string]any, error) {
	routeIDs := map[int64]int64{}
	gearIDs := map[int64]int64{}
	added := map[string]int{}
	for _, r := range doc.Routes {
		src, ref := r.Source, r.SourceRef
		if ref == "" {
			src, ref = "import", fmt.Sprintf("route:%d:%s", r.ID, r.CreatedAt)
		}
		if have, err := s.store.RouteBySource(ctx, uid, src, ref); err == nil {
			routeIDs[r.ID] = have.ID
			continue
		}
		old := r.ID
		wps := r.Waypoints
		r.Source, r.SourceRef = src, ref
		r.Difficulty = map[bool]string{true: r.Difficulty, false: ""}[oneOf(r.Difficulty, store.Difficulties)]
		if !oneOf(r.Activity, store.Activities) {
			r.Activity = "other"
		}
		prepRoute(r)
		if err := s.store.CreateRoute(ctx, uid, r); err != nil {
			return nil, err
		}
		routeIDs[old] = r.ID
		added["routes"]++
		for _, w := range wps {
			w.RouteID = r.ID
			if !oneOf(w.Kind, store.WaypointKinds) {
				w.Kind = "other"
			}
			placeWaypoint(r, w)
			_ = s.store.CreateWaypoint(ctx, uid, w)
		}
	}
	for _, g := range doc.Gear {
		if id, ok := s.store.GearByName(ctx, uid, g.Name); ok {
			gearIDs[g.ID] = id
			continue
		}
		old := g.ID
		if !oneOf(g.Category, store.GearKinds) {
			g.Category = "other"
		}
		if err := s.store.CreateGear(ctx, uid, g); err != nil {
			return nil, err
		}
		gearIDs[old] = g.ID
		added["gear"]++
	}
	have, _ := s.store.ListPacks(ctx, uid)
	names := map[string]bool{}
	for _, p := range have {
		names[strings.ToLower(p.Name)] = true
	}
	for _, p := range doc.Packs {
		if names[strings.ToLower(p.Name)] {
			continue
		}
		var items []store.PackItem
		for _, it := range p.Items {
			if id := gearIDs[it.GearID]; id > 0 {
				items = append(items, store.PackItem{GearID: id, Qty: it.Qty})
			}
		}
		np := &store.Pack{Name: p.Name, Activity: p.Activity, Note: p.Note}
		if err := s.store.CreatePack(ctx, uid, np); err != nil {
			return nil, err
		}
		_ = s.store.SetPackItems(ctx, uid, np.ID, items)
		added["packs"]++
	}
	for _, o := range doc.Outings {
		src, ref := o.Source, o.SourceRef
		if ref == "" {
			src, ref = "import", fmt.Sprintf("outing:%d:%s", o.ID, o.CreatedAt)
		}
		if _, err := s.store.OutingBySource(ctx, uid, src, ref); err == nil {
			continue
		}
		o.Source, o.SourceRef = src, ref
		if o.RouteID != nil {
			if id := routeIDs[*o.RouteID]; id > 0 {
				o.RouteID = &id
			} else {
				o.RouteID = nil
			}
		}
		if !oneOf(o.Status, store.OutingStates) {
			o.Status = store.OutingPlanned
		}
		if !oneOf(o.Activity, store.Activities) {
			o.Activity = "other"
		}
		o.CalRef, o.HealthRef, o.NoteRef = "", "", "" // those belong to the other box's apps
		o.Safety = false
		items := o.Items
		o.ActualTrack = geo.Thin(o.ActualTrack, maxTrack)
		if err := s.store.CreateOuting(ctx, uid, o); err != nil {
			return nil, err
		}
		added["outings"]++
		for _, it := range items {
			it.OutingID = o.ID
			if it.GearID != nil {
				if id := gearIDs[*it.GearID]; id > 0 {
					it.GearID = &id
				} else {
					it.GearID = nil
				}
			}
			_ = s.store.AddOutingItem(ctx, uid, it)
		}
	}
	return map[string]any{"added": added}, nil
}
