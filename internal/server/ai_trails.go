package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// ---- Pocket AI (or the person's own OpenAI key) ----
//
// pack: what to take on an outing, from the route, the day's weather and
// the person's own gear; report: a short account of how an outing went;
// describe: a route described from its numbers and its spots.

func (s *Server) routeBrief(r *store.Route, st store.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Route: %s (%s)\nDistance: %s, climb %s up and %s down, about %s, %s.\n", r.Name, activityWords[r.Activity], fmtDist(r.DistanceM, st.Units),
		fmtHeight(r.GainM, st.Units), fmtHeight(r.LossM, st.Units), fmtMinutes(r.Minutes), levelWords[r.Level])
	if r.MinEle != nil && r.MaxEle != nil {
		fmt.Fprintf(&b, "Height: from %s to %s.\n", fmtHeight(*r.MinEle, st.Units), fmtHeight(*r.MaxEle, st.Units))
	}
	if r.Loop {
		b.WriteString("It is a loop.\n")
	}
	if r.Region != "" {
		b.WriteString("Region: " + r.Region + "\n")
	}
	if r.StartName != "" {
		b.WriteString("Starts at: " + r.StartName + "\n")
	}
	if len(r.Waypoints) > 0 {
		b.WriteString("Along the way:\n")
		for _, w := range r.Waypoints {
			fmt.Fprintf(&b, "- at %s: %s %s %s\n", fmtDist(w.AtM, st.Units), kindWords[w.Kind], w.Name, clip(w.Note, 200))
		}
	}
	if r.Description != "" {
		b.WriteString("Notes: " + clip(r.Description, 1500) + "\n")
	}
	return b.String()
}

func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return
	}
	var in struct {
		Want     string `json:"want"` // pack | report | describe
		OutingID int64  `json:"outing_id"`
		RouteID  int64  `json:"route_id"`
		Hint     string `json:"hint"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusBadRequest, firstNonEmpty(why, "AI is not set up (Settings → AI)"))
		return
	}
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	default:
		writeErr(w, http.StatusTooManyRequests, "the AI is busy; try again in a moment")
		return
	}
	hint := clip(strings.TrimSpace(in.Hint), 1000)
	switch in.Want {
	case "describe":
		rt, err := s.store.GetRoute(ctx, uid, in.RouteID)
		if err != nil {
			s.fail(w, err, "route")
			return
		}
		decorateRoute(rt, st.PacePct)
		sys := "You write short, practical route descriptions for a hiking and cycling app, in plain everyday English, no marketing language, no invented facts: use only what you are given. Two short paragraphs at most: what the route is like (length, climb, terrain you can infer from the numbers), and what to know (water, huts, tricky bits, best for whom). No headings, no lists."
		text, err := cl.Complete(ctx, sys, s.routeBrief(rt, st)+"\n"+hint, false)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(text)})
	case "report":
		page, err := s.outingPage(ctx, uid, in.OutingID)
		if err != nil {
			s.fail(w, err, "outing")
			return
		}
		o := page["outing"].(*store.Outing)
		rt, _ := page["route"].(*store.Route)
		var b strings.Builder
		b.WriteString(s.outingMarkdown(o, rt, st))
		var c geo.Comparison
		if len(o.Actual) > 0 && json.Unmarshal(o.Actual, &c) == nil && c.PlannedM > 0 {
			fmt.Fprintf(&b, "\nPlanned %s; covered %d%% of the planned way; furthest off it %s; moving speed %d%% of the usual.\n", fmtDist(c.PlannedM, st.Units), c.CoveredPct, fmtHeight(c.MaxOffM, st.Units), c.SpeedPct)
		}
		var wd dayWeather
		if len(o.Weather) > 0 && json.Unmarshal(o.Weather, &wd) == nil {
			fmt.Fprintf(&b, "\nWeather that day: %s", firstNonEmpty(wd.Text, wd.Condition))
		}
		if hint != "" {
			b.WriteString("\nWhat the person says: " + hint)
		}
		sys := "You write a short personal account of a hike or ride for someone's own diary, in the first person, plain and warm, no exaggeration and nothing invented: only what the notes say. Up to 120 words. End with one line starting 'Next time:' with a practical tip if the notes suggest one."
		text, err := cl.Complete(ctx, sys, b.String(), false)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(text)})
	case "pack":
		page, err := s.outingPage(ctx, uid, in.OutingID)
		if err != nil {
			s.fail(w, err, "outing")
			return
		}
		o := page["outing"].(*store.Outing)
		rt, _ := page["route"].(*store.Route)
		gear, _ := s.store.ListGear(ctx, uid, false, "")
		var b strings.Builder
		fmt.Fprintf(&b, "Outing: %s on %s, starting %s, about %s.\n", activityWords[o.Activity], o.Date, firstNonEmpty(o.StartTime, "in the morning"), fmtMinutes(o.Minutes))
		if rt != nil {
			b.WriteString(s.routeBrief(rt, st))
		}
		for _, a := range page["advice"].([]advice) {
			b.WriteString("Note: " + a.Text + "\n")
		}
		var wd dayWeather
		if len(o.Weather) > 0 && json.Unmarshal(o.Weather, &wd) == nil {
			fmt.Fprintf(&b, "Weather: %s", firstNonEmpty(wd.Text, wd.Condition))
			if wd.TMin != nil && wd.TMax != nil {
				fmt.Fprintf(&b, ", %.0f to %.0f °C", *wd.TMin, *wd.TMax)
			}
			b.WriteString("\n")
		}
		if n := len(o.People); n > 0 {
			fmt.Fprintf(&b, "Group of %d.\n", n+1)
		}
		b.WriteString("Already on the list: ")
		for _, it := range o.Items {
			b.WriteString(it.Name + "; ")
		}
		b.WriteString("\nThe person's own gear (use these exact names when they fit):\n")
		for _, g := range gear {
			b.WriteString("- " + g.Name + "\n")
		}
		if hint != "" {
			b.WriteString("They add: " + hint + "\n")
		}
		sys := `You help pack for a day outdoors. Suggest what to take that is NOT already on the list, sensible for this route, length and weather — safety first (water, layers, light, first aid), no silly extras. Prefer the person's own gear names. Answer JSON: {"items":[{"name":"…","category":"pack|shelter|sleep|clothing|footwear|cook|water|food|navigation|safety|first_aid|electronics|bike|snow|other","why":"a few words"}]} with at most 12 items.`
		var out struct {
			Items []struct {
				Name     string `json:"name"`
				Category string `json:"category"`
				Why      string `json:"why"`
			} `json:"items"`
		}
		if err := cl.JSON(ctx, sys, b.String(), &out); err != nil {
			writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
			return
		}
		byName := map[string]*store.Gear{}
		for _, g := range gear {
			byName[strings.ToLower(g.Name)] = g
		}
		res := []map[string]any{}
		for _, it := range out.Items {
			name := clip(strings.TrimSpace(it.Name), 200)
			if name == "" {
				continue
			}
			cat := it.Category
			if !oneOf(cat, store.GearKinds) {
				cat = guessGearKind(name)
			}
			m := map[string]any{"name": name, "category": cat, "why": clip(it.Why, 200)}
			if g := byName[strings.ToLower(name)]; g != nil {
				m["gear_id"], m["weight_g"] = g.ID, g.WeightG
			}
			res = append(res, m)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": res})
	default:
		writeErr(w, http.StatusBadRequest, "want is pack, report or describe")
	}
}

var _ = context.Background
