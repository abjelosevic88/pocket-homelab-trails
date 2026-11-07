package server

import (
	"bytes"
	"encoding/csv"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

type gearInput struct {
	Name       *string `json:"name"`
	Category   *string `json:"category"`
	WeightG    *int    `json:"weight_g"`
	Worn       *bool   `json:"worn"`
	Consumable *bool   `json:"consumable"`
	Note       *string `json:"note"`
	Archived   *bool   `json:"archived"`
}

func (in gearInput) apply(g *store.Gear) string {
	trimP(&g.Name, in.Name)
	if in.Category != nil {
		if !oneOf(*in.Category, store.GearKinds) {
			return "unknown kind of gear"
		}
		g.Category = *in.Category
	}
	if in.WeightG != nil {
		if *in.WeightG < 0 || *in.WeightG > 100000 {
			return "the weight is 0 to 100 kg"
		}
		g.WeightG = *in.WeightG
	}
	if in.Worn != nil {
		g.Worn = *in.Worn
	}
	if in.Consumable != nil {
		g.Consumable = *in.Consumable
	}
	if in.Note != nil {
		g.Note = clip(strings.TrimSpace(*in.Note), 2000)
	}
	if in.Archived != nil {
		g.Archived = *in.Archived
	}
	if g.Name == "" {
		return "name the gear"
	}
	g.Name = clip(g.Name, 200)
	return ""
}

func (s *Server) handleListGear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListGear(ctx, actorID(ctx), queryBool(r, "archived"), r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, err, "gear")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateGear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in gearInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	g := &store.Gear{Category: "other"}
	if msg := in.apply(g); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.Category == nil {
		g.Category = guessGearKind(g.Name)
	}
	if err := s.store.CreateGear(ctx, actorID(ctx), g); err != nil {
		s.fail(w, err, "add gear")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := s.store.GetGear(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "gear")
		return
	}
	var in gearInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(g); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateGear(ctx, g); err != nil {
		s.fail(w, err, "save gear")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGear(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGear(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete gear")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- packing lists ----

func (s *Server) handleListPacks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListPacks(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "packing lists")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type packInput struct {
	Name     *string          `json:"name"`
	Activity *string          `json:"activity"`
	Note     *string          `json:"note"`
	Items    []store.PackItem `json:"items"`
}

func (in packInput) apply(p *store.Pack) string {
	trimP(&p.Name, in.Name)
	if in.Activity != nil {
		if *in.Activity != "" && !oneOf(*in.Activity, store.Activities) {
			return "unknown activity"
		}
		p.Activity = *in.Activity
	}
	if in.Note != nil {
		p.Note = clip(strings.TrimSpace(*in.Note), 2000)
	}
	if p.Name == "" {
		return "name the packing list"
	}
	p.Name = clip(p.Name, 200)
	return ""
}

func (s *Server) handleCreatePack(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in packInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.Pack{}
	if msg := in.apply(p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreatePack(ctx, uid, p); err != nil {
		s.fail(w, err, "add packing list")
		return
	}
	if keys["items"] {
		if err := s.store.SetPackItems(ctx, uid, p.ID, in.Items); err != nil {
			s.fail(w, err, "packing list")
			return
		}
	}
	out, err := s.store.GetPack(ctx, uid, p.ID)
	if err != nil {
		s.fail(w, err, "packing list")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdatePack(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPack(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "packing list")
		return
	}
	var in packInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	p.UserID = uid
	if err := s.store.UpdatePack(ctx, p); err != nil {
		s.fail(w, err, "save packing list")
		return
	}
	if keys["items"] {
		if err := s.store.SetPackItems(ctx, uid, p.ID, in.Items); err != nil {
			s.fail(w, err, "packing list")
			return
		}
	}
	out, err := s.store.GetPack(ctx, uid, p.ID)
	if err != nil {
		s.fail(w, err, "packing list")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeletePack(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePack(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete packing list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- bringing gear in ----

// handleImportGear reads a LighterPack CSV (or any CSV with a name, a
// weight and maybe a category): the gear is added (or found by name), and
// a packing list named after the file holds it all.
func (s *Server) handleImportGear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	name := firstNonEmpty(r.URL.Query().Get("name"), "Imported list")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(s.cfg.MaxBodyBytes); err != nil || len(r.MultipartForm.File["file"]) == 0 {
			writeErr(w, http.StatusBadRequest, "choose a CSV file")
			return
		}
		fh := r.MultipartForm.File["file"][0]
		name = strings.TrimSuffix(fh.Filename, fileExtOf(fh.Filename))
		var err error
		if data, err = readPart(fh, s.cfg.MaxBodyBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "the upload could not be read")
			return
		}
	} else {
		var err error
		if data, err = s.readBody(r); err != nil {
			writeErr(w, http.StatusRequestEntityTooLarge, "the file is too big")
			return
		}
	}
	res, err := s.importGearCSV(r, uid, name, data)
	if err != nil {
		s.userFail(w, err, "import gear")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func fileExtOf(n string) string {
	if i := strings.LastIndexByte(n, '.'); i > 0 {
		return n[i:]
	}
	return ""
}

func (s *Server) importGearCSV(r *http.Request, uid int64, listName string, data []byte) (map[string]any, error) {
	ctx := r.Context()
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	if bytes.Count(data[:min(len(data), 2000)], []byte(";")) > bytes.Count(data[:min(len(data), 2000)], []byte(",")) {
		rd.Comma = ';'
	}
	head, err := rd.Read()
	if err != nil {
		return nil, &userErr{"the file is empty or not a CSV"}
	}
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	col := func(rec []string, names ...string) string {
		for _, n := range names {
			if i, ok := idx[n]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}
	if _, ok := idx["item name"]; !ok {
		if _, ok := idx["name"]; !ok {
			return nil, &userErr{"the CSV needs a column called “Item Name” (LighterPack) or “Name”"}
		}
	}
	var items []store.PackItem
	added, found := 0, 0
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &userErr{"the CSV could not be read: " + err.Error()}
		}
		name := col(rec, "item name", "name")
		if name == "" {
			continue
		}
		qty, _ := strconv.Atoi(col(rec, "qty", "quantity"))
		w, _ := strconv.ParseFloat(strings.ReplaceAll(col(rec, "weight", "weight (g)", "grams"), ",", "."), 64)
		grams := toGrams(w, col(rec, "unit"))
		truthy := func(v string) bool {
			v = strings.ToLower(v)
			return v != "" && v != "0" && v != "false" && v != "no"
		}
		g := &store.Gear{Name: clip(name, 200), Category: guessGearKind(col(rec, "category") + " " + name), WeightG: grams,
			Worn: truthy(col(rec, "worn")), Consumable: truthy(col(rec, "consumable")), Note: clip(col(rec, "desc", "description", "note"), 2000)}
		if id, ok := s.store.GearByName(ctx, uid, g.Name); ok {
			found++
			items = append(items, store.PackItem{GearID: id, Qty: max(qty, 1)})
			continue
		}
		if err := s.store.CreateGear(ctx, uid, g); err != nil {
			return nil, err
		}
		added++
		items = append(items, store.PackItem{GearID: g.ID, Qty: max(qty, 1)})
	}
	if len(items) == 0 {
		return nil, &userErr{"no gear found in the file"}
	}
	p := &store.Pack{Name: clip(listName, 200)}
	if err := s.store.CreatePack(ctx, uid, p); err != nil {
		return nil, err
	}
	if err := s.store.SetPackItems(ctx, uid, p.ID, items); err != nil {
		return nil, err
	}
	return map[string]any{"added": added, "found": found, "pack_id": p.ID}, nil
}

func toGrams(w float64, unit string) int {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "oz":
		w *= 28.3495
	case "lb", "lbs":
		w *= 453.592
	case "kg":
		w *= 1000
	}
	return int(math.Round(w))
}

// guessGearKind sorts gear by its name or category.
func guessGearKind(s string) string {
	s = strings.ToLower(s)
	has := func(w ...string) bool {
		for _, x := range w {
			if strings.Contains(s, x) {
				return true
			}
		}
		return false
	}
	switch {
	case has("backpack", "rucksack", "pack ", "daypack", "pack liner"):
		return "pack"
	case has("tent", "tarp", "shelter", "bivy", "stake", "peg"):
		return "shelter"
	case has("sleep", "quilt", "pad", "mat ", "pillow"):
		return "sleep"
	case has("boot", "shoe", "sock", "gaiter", "footwear"):
		return "footwear"
	case has("jacket", "fleece", "shirt", "pants", "trousers", "shorts", "hat", "cap", "glove", "buff", "layer", "cloth", "rain"):
		return "clothing"
	case has("stove", "pot", "spork", "mug", "fuel", "lighter", "cook", "kitchen"):
		return "cook"
	case has("bottle", "bladder", "filter", "water", "flask"):
		return "water"
	case has("food", "snack", "bar", "sandwich", "lunch", "nuts", "gel"):
		return "food"
	case has("map", "compass", "gps", "navigation"):
		return "navigation"
	case has("first aid", "first-aid", "plaster", "blister", "medic"):
		return "first_aid"
	case has("headlamp", "head torch", "torch", "whistle", "emergency", "blanket", "knife", "safety", "helmet"):
		return "safety"
	case has("phone", "battery", "power bank", "charger", "cable", "camera", "watch", "electronic"):
		return "electronics"
	case has("pump", "tube", "multitool", "chain", "bike", "lock", "light"):
		return "bike"
	case has("crampon", "ice axe", "microspike", "avalanche", "probe", "shovel", "skin", "snow"):
		return "snow"
	}
	return "other"
}

// handleGearFromInventory adds the picked Pocket Inventory items as gear
// (once each: picked again, the gear is found, not doubled).
func (s *Server) handleGearFromInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	added := 0
	for _, it := range in.Items {
		if it.ID == "" || strings.TrimSpace(it.Name) == "" {
			continue
		}
		if _, ok := s.store.GearByInventory(ctx, uid, it.ID); ok {
			continue
		}
		g := &store.Gear{Name: clip(strings.TrimSpace(it.Name), 200), Category: guessGearKind(it.Name), InventoryRef: it.ID}
		if err := s.store.CreateGear(ctx, uid, g); err != nil {
			s.fail(w, err, "add gear")
			return
		}
		added++
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}
