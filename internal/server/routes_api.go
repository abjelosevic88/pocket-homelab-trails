package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// routeInput is what a route form (or another app) sends; nil = not sent.
type routeInput struct {
	Name        *string         `json:"name"`
	Activity    *string         `json:"activity"`
	Difficulty  *string         `json:"difficulty"`
	Region      *string         `json:"region"`
	Description *string         `json:"description"`
	Tags        []string        `json:"tags"`
	Track       []store.Pt      `json:"track"`
	Controls    [][]float64     `json:"controls"`
	Drawn       *string         `json:"drawn"`
	MinutesSet  *int            `json:"minutes_set"`
	StartName   *string         `json:"start_name"`
	LinkURL     *string         `json:"link_url"`
	Favourite   *bool           `json:"favourite"`
	Archived    *bool           `json:"archived"`
	SourceRef   *string         `json:"source_ref"`
	Waypoints   []waypointInput `json:"waypoints"`
	Heights     bool            `json:"fill_heights"` // look heights up when the track has none
}

func (in routeInput) apply(r *store.Route, keys map[string]bool) string {
	trimP(&r.Name, in.Name)
	if in.Activity != nil {
		if !oneOf(*in.Activity, store.Activities) {
			return "unknown activity"
		}
		r.Activity = *in.Activity
	}
	if in.Difficulty != nil {
		if !oneOf(*in.Difficulty, store.Difficulties) {
			return "the difficulty is easy, moderate, hard or expert"
		}
		r.Difficulty = *in.Difficulty
	}
	trimP(&r.Region, in.Region)
	if in.Description != nil {
		r.Description = clip(strings.TrimSpace(*in.Description), 20000)
	}
	if keys["tags"] {
		r.Tags = store.CleanTags(in.Tags)
	}
	if keys["track"] {
		var tr []store.Pt
		for _, p := range in.Track {
			if p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180 && !(p.Lat == 0 && p.Lng == 0) {
				tr = append(tr, p)
			}
		}
		if len(tr) > 200000 {
			return "the track has too many points"
		}
		r.Track = tr
	}
	if keys["controls"] {
		if len(in.Controls) > 200 {
			return "too many drawn points"
		}
		r.Controls = in.Controls
	}
	if in.Drawn != nil {
		if *in.Drawn != "" && *in.Drawn != "straight" && *in.Drawn != "paths" {
			return "drawn is straight or paths"
		}
		r.Drawn = *in.Drawn
	}
	if in.MinutesSet != nil {
		if *in.MinutesSet < 0 || *in.MinutesSet > 60*24*14 {
			return "the time is 0 to two weeks"
		}
		r.MinutesSet = *in.MinutesSet
	}
	trimP(&r.StartName, in.StartName)
	if in.LinkURL != nil {
		u, ok := cleanURL(*in.LinkURL)
		if !ok {
			return "the link must start with http:// or https://"
		}
		r.LinkURL = u
	}
	if in.Favourite != nil {
		r.Favourite = *in.Favourite
	}
	if in.Archived != nil {
		r.Archived = *in.Archived
	}
	if in.SourceRef != nil {
		r.SourceRef = clip(strings.TrimSpace(*in.SourceRef), 200)
	}
	r.Name = clip(r.Name, 200)
	return ""
}

func (s *Server) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	list, err := s.store.ListRoutes(ctx, uid, store.RouteFilter{Activity: q.Get("activity"), Q: q.Get("q"), Archived: queryBool(r, "archived")})
	if err != nil {
		s.fail(w, err, "routes")
		return
	}
	st := s.settingsOf(ctx, uid)
	for _, rt := range list {
		decorateRoute(rt, st.PacePct)
	}
	writeJSON(w, http.StatusOK, list)
}

// routePage is a route with what its page needs.
func (s *Server) routePage(ctx context.Context, uid, id int64) (map[string]any, error) {
	rt, err := s.store.GetRoute(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	st := s.settingsOf(ctx, uid)
	decorateRoute(rt, st.PacePct)
	outs, facts, err := s.store.ListOutings(ctx, uid, store.OutingFilter{RouteID: id, Desc: true})
	if err != nil {
		return nil, err
	}
	for i, o := range outs {
		decorateOuting(o, facts[i], st.PacePct)
	}
	return map[string]any{"route": rt, "outings": outs}, nil
}

func (s *Server) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	out, err := s.routePage(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	if linkApp(r.Context()) != "" || queryBool(r, "plain") {
		writeJSON(w, http.StatusOK, out["route"])
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in routeInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := s.settingsOf(ctx, uid)
	rt := &store.Route{Activity: firstNonEmpty(st.DefaultActivity, "hike"), Source: "manual"}
	if app := linkApp(ctx); app != "" {
		rt.Source = app
	}
	if msg := in.apply(rt, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if rt.SourceRef != "" {
		if have, err := s.store.RouteBySource(ctx, uid, rt.Source, rt.SourceRef); err == nil {
			decorateRoute(have, st.PacePct)
			writeJSON(w, http.StatusOK, have)
			return
		}
	}
	if len(rt.Track) < 2 {
		writeErr(w, http.StatusBadRequest, "a route needs a track of at least two points")
		return
	}
	if rt.Name == "" {
		rt.Name = activityWords[rt.Activity]
	}
	if rt.Drawn != "" {
		rt.Source = "drawn"
	}
	if (in.Heights || rt.Drawn != "") && !geo.HasEle(rt.Track) && s.online(st) {
		if rt.Drawn == "straight" {
			rt.Track = densify(rt.Track, 60)
		}
		_ = s.fillHeights(ctx, geo.Thin(rt.Track, 1500))
	}
	prepRoute(rt)
	if err := s.store.CreateRoute(ctx, uid, rt); err != nil {
		s.fail(w, err, "create route")
		return
	}
	for _, wi := range in.Waypoints {
		wp := &store.Waypoint{RouteID: rt.ID, Kind: "other"}
		if msg := wi.apply(wp); msg != "" {
			continue
		}
		placeWaypoint(rt, wp)
		_ = s.store.CreateWaypoint(ctx, uid, wp)
	}
	s.publish(ctx, uid, "trails.route_saved", "route:"+itoa(rt.ID), s.routeEvent(rt, st))
	out, _ := s.store.GetRoute(ctx, uid, rt.ID)
	decorateRoute(out, st.PacePct)
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) routeEvent(rt *store.Route, st store.Settings) map[string]any {
	decorateRoute(rt, st.PacePct)
	return map[string]any{"route_id": rt.ID, "name": rt.Name, "activity": rt.Activity, "distance_m": rt.DistanceM, "gain_m": rt.GainM,
		"minutes": rt.Minutes, "level": rt.Level, "url": s.link(fmt.Sprintf("/routes/%d", rt.ID))}
}

func (s *Server) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rt, err := s.store.GetRoute(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	if app := linkApp(ctx); app != "" && rt.Source != app {
		writeErr(w, http.StatusForbidden, "this route was not added by "+app)
		return
	}
	var in routeInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.SourceRef = nil
	if msg := in.apply(rt, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if rt.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the route a name")
		return
	}
	st := s.settingsOf(ctx, uid)
	if keys["track"] {
		if len(rt.Track) < 2 {
			writeErr(w, http.StatusBadRequest, "a route needs a track of at least two points")
			return
		}
		if (in.Heights || rt.Drawn != "") && !geo.HasEle(rt.Track) && s.online(st) {
			if rt.Drawn == "straight" {
				rt.Track = densify(rt.Track, 60)
			}
			_ = s.fillHeights(ctx, geo.Thin(rt.Track, 1500))
		}
		prepRoute(rt)
		for _, wp := range rt.Waypoints {
			wp.Ele = nil
			placeWaypoint(rt, wp)
			_ = s.store.UpdateWaypoint(ctx, wp)
		}
	}
	if err := s.store.UpdateRoute(ctx, rt); err != nil {
		s.fail(w, err, "save route")
		return
	}
	out, err := s.routePage(ctx, uid, rt.ID)
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	if linkApp(ctx) != "" {
		writeJSON(w, http.StatusOK, out["route"])
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if app := linkApp(ctx); app != "" {
		rt, err := s.store.GetRoute(ctx, uid, idParam(r, "id"))
		if err != nil {
			s.fail(w, err, "route")
			return
		}
		if rt.Source != app {
			writeErr(w, http.StatusForbidden, "this route was not added by "+app)
			return
		}
	}
	if err := s.store.DeleteRoute(ctx, uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete route")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReverseRoute walks a route the other way round.
func (s *Server) handleReverseRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rt, err := s.store.GetRoute(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	for i, j := 0, len(rt.Track)-1; i < j; i, j = i+1, j-1 {
		rt.Track[i], rt.Track[j] = rt.Track[j], rt.Track[i]
	}
	for i, j := 0, len(rt.Controls)-1; i < j; i, j = i+1, j-1 {
		rt.Controls[i], rt.Controls[j] = rt.Controls[j], rt.Controls[i]
	}
	prepRoute(rt)
	for _, wp := range rt.Waypoints {
		placeWaypoint(rt, wp)
		_ = s.store.UpdateWaypoint(ctx, wp)
	}
	if err := s.store.UpdateRoute(ctx, rt); err != nil {
		s.fail(w, err, "save route")
		return
	}
	out, _ := s.routePage(ctx, uid, rt.ID)
	writeJSON(w, http.StatusOK, out)
}

// handleRouteGPX downloads a route as GPX (for OsmAnd, Organic Maps,
// Komoot, a Garmin: the way to have it offline on the trail).
func (s *Server) handleRouteGPX(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := s.store.GetRoute(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	w.Header().Set("Content-Type", "application/gpx+xml")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName(rt.Name)+`.gpx"`)
	_ = geo.WriteGPX(w, rt.Name, rt.Description, rt.Activity, rt.Track, rt.Waypoints)
}

var reFileBad = regexp.MustCompile(`[^\p{L}\p{N} ._-]+`)

func fileName(n string) string {
	n = strings.TrimSpace(reFileBad.ReplaceAllString(n, ""))
	if n == "" {
		return "route"
	}
	return clip(n, 80)
}

// ---- importing files ----

// handleImportRoutes takes GPX, KML, TCX and GeoJSON files (a form with
// one or more "file" parts, or the file as the body with ?name=): every
// track becomes a route. The same file twice gives back the same routes.
func (s *Server) handleImportRoutes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	type file struct {
		name string
		data []byte
	}
	var files []file
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(s.cfg.MaxBodyBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "the upload could not be read")
			return
		}
		for _, fh := range r.MultipartForm.File["file"] {
			b, err := readPart(fh, s.cfg.MaxBodyBytes)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "the upload could not be read")
				return
			}
			files = append(files, file{fh.Filename, b})
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusRequestEntityTooLarge, "the file is too big")
			return
		}
		files = append(files, file{firstNonEmpty(r.URL.Query().Get("name"), "route.gpx"), b})
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "choose a GPX, KML, TCX or GeoJSON file")
		return
	}
	activity := r.URL.Query().Get("activity")
	if !oneOf(activity, store.Activities) {
		activity = ""
	}
	made := []*store.Route{}
	var problems []string
	for _, f := range files {
		parsed, err := geo.ParseFile(f.name, f.data)
		if err != nil {
			problems = append(problems, f.name+": "+err.Error())
			continue
		}
		for i, p := range parsed {
			ref := fmt.Sprintf("%016x:%d", fnv64(f.data), i)
			if have, err := s.store.RouteBySource(ctx, uid, "gpx", ref); err == nil {
				decorateRoute(have, st.PacePct)
				have.Track = nil
				made = append(made, have)
				continue
			}
			rt := &store.Route{Name: p.Name, Description: p.Desc, Activity: firstNonEmpty(activity, p.Activity, st.DefaultActivity, "hike"), Track: p.Track,
				LinkURL: p.Link, Source: "gpx", SourceRef: ref}
			if u, ok := cleanURL(rt.LinkURL); ok {
				rt.LinkURL = u
			} else {
				rt.LinkURL = ""
			}
			if !geo.HasEle(rt.Track) && s.online(st) {
				rt.Track = geo.Thin(rt.Track, 1500)
				if err := s.fillHeights(ctx, rt.Track); err != nil {
					s.log.Debug("import: heights", "err", err)
				}
			}
			prepRoute(rt)
			if err := s.store.CreateRoute(ctx, uid, rt); err != nil {
				problems = append(problems, f.name+": "+err.Error())
				continue
			}
			for _, pw := range p.Waypoints {
				wp := &store.Waypoint{RouteID: rt.ID, Kind: pw.Kind, Name: pw.Name, Note: pw.Note, Lat: pw.Lat, Lng: pw.Lng, Ele: pw.Ele}
				placeWaypoint(rt, wp)
				_ = s.store.CreateWaypoint(ctx, uid, wp)
			}
			s.publish(ctx, uid, "trails.route_saved", "route:"+itoa(rt.ID), s.routeEvent(rt, st))
			decorateRoute(rt, st.PacePct)
			rt.Track = nil
			made = append(made, rt)
		}
	}
	if len(made) == 0 && len(problems) > 0 {
		writeErr(w, http.StatusBadRequest, strings.Join(problems, "; "))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"routes": made, "problems": problems})
}

func readPart(fh *multipart.FileHeader, max int64) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("too big")
	}
	return b, nil
}

// ---- waypoints ----

type waypointInput struct {
	Kind     *string  `json:"kind"`
	Name     *string  `json:"name"`
	Note     *string  `json:"note"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Ele      *float64 `json:"ele"`
	PlaceRef *string  `json:"place_ref"`
}

func (in waypointInput) apply(w *store.Waypoint) string {
	if in.Kind != nil {
		if !oneOf(*in.Kind, store.WaypointKinds) {
			return "unknown kind of spot"
		}
		w.Kind = *in.Kind
	}
	trimP(&w.Name, in.Name)
	trimP(&w.Note, in.Note)
	if in.Lat != nil {
		w.Lat = *in.Lat
	}
	if in.Lng != nil {
		w.Lng = *in.Lng
	}
	if in.Ele != nil {
		w.Ele = in.Ele
	}
	trimP(&w.PlaceRef, in.PlaceRef)
	if w.Lat < -90 || w.Lat > 90 || w.Lng < -180 || w.Lng > 180 || (w.Lat == 0 && w.Lng == 0) {
		return "put the spot on the map"
	}
	w.Name, w.Note = clip(w.Name, 200), clip(w.Note, 2000)
	return ""
}

func (s *Server) handleCreateWaypoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rt, err := s.store.GetRoute(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	var in waypointInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	wp := &store.Waypoint{RouteID: rt.ID, Kind: "other"}
	if msg := in.apply(wp); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if wp.Kind == "other" && wp.Name != "" {
		wp.Kind = geo.GuessKind(wp.Name)
	}
	placeWaypoint(rt, wp)
	if err := s.store.CreateWaypoint(ctx, uid, wp); err != nil {
		s.fail(w, err, "add spot")
		return
	}
	writeJSON(w, http.StatusCreated, wp)
}

func (s *Server) handleUpdateWaypoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wp, err := s.store.GetWaypoint(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "spot")
		return
	}
	var in waypointInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	moved := in.Lat != nil || in.Lng != nil
	if msg := in.apply(wp); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if moved {
		if rt, err := s.store.GetRoute(ctx, uid, wp.RouteID); err == nil {
			if in.Ele == nil {
				wp.Ele = nil
			}
			placeWaypoint(rt, wp)
		}
	}
	if err := s.store.UpdateWaypoint(ctx, wp); err != nil {
		s.fail(w, err, "save spot")
		return
	}
	writeJSON(w, http.StatusOK, wp)
}

func (s *Server) handleDeleteWaypoint(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteWaypoint(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete spot")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- drawing: heights and paths for the planner ----

// handleDrawPreview turns drawn points into a track: straight lines, or
// along paths (BRouter), with heights when online lookups are on. Nothing
// is saved; the planner shows the numbers as you draw.
func (s *Server) handleDrawPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	var in struct {
		Controls [][]float64 `json:"controls"`
		Mode     string      `json:"mode"` // straight | paths
		Activity string      `json:"activity"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var track []store.Pt
	heights := ""
	if in.Mode == "paths" {
		if !s.online(st) {
			s.userFail(w, errOffline, "paths")
			return
		}
		var err error
		if track, err = s.followPaths(ctx, in.Controls, in.Activity); err != nil {
			s.userFail(w, err, "paths")
			return
		}
	} else {
		for _, c := range in.Controls {
			if len(c) >= 2 {
				track = append(track, store.Pt{Lat: c[0], Lng: c[1]})
			}
		}
		if len(track) >= 2 && s.online(st) {
			track = densify(track, 60)
			if err := s.fillHeights(ctx, geo.Thin(track, 1500)); err != nil {
				heights = "heights could not be looked up: " + err.Error()
			}
		} else if !s.online(st) {
			heights = "heights are looked up online (switched off in Settings → General)"
		}
	}
	stt := geo.Measure(track)
	pace := st.PacePct
	out := map[string]any{"track": geo.Thin(track, maxTrack), "distance_m": math.Round(stt.DistanceM), "gain_m": math.Round(stt.GainM), "loss_m": math.Round(stt.LossM),
		"min_ele": stt.MinEle, "max_ele": stt.MaxEle, "loop": stt.Loop, "minutes": geo.Minutes(in.Activity, stt.DistanceM, stt.GainM, stt.LossM, pace),
		"level": geo.Level(in.Activity, stt.DistanceM, stt.GainM), "heights_note": heights}
	writeJSON(w, http.StatusOK, out)
}

// handlePlaceSearch finds a mountain, a village or a car park by name.
func (s *Server) handlePlaceSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := s.settingsOf(ctx, actorID(ctx))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if !s.online(st) {
		s.userFail(w, errOffline, "search")
		return
	}
	out, err := s.searchPlaces(ctx, clip(q, 200))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "place search: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// fnv64 is a quick fingerprint of a file (the same upload twice).
func fnv64(b []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}
