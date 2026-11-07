package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// yearTotals adds up a year's finished outings.
type yearTotals struct {
	Year       int                       `json:"year"`
	Outings    int                       `json:"outings"`
	DistanceM  float64                   `json:"distance_m"`
	GainM      float64                   `json:"gain_m"`
	Minutes    int                       `json:"minutes"`
	ByActivity map[string]*activityTotal `json:"by_activity"`
	ByMonth    []monthTotal              `json:"by_month"`
	Summits    int                       `json:"summits"`
	Highest    *float64                  `json:"highest"`
	Longest    *store.Outing             `json:"longest,omitempty"`
}

type activityTotal struct {
	Outings   int     `json:"outings"`
	DistanceM float64 `json:"distance_m"`
	GainM     float64 `json:"gain_m"`
	Minutes   int     `json:"minutes"`
}

type monthTotal struct {
	Month     int     `json:"month"`
	Outings   int     `json:"outings"`
	DistanceM float64 `json:"distance_m"`
	GainM     float64 `json:"gain_m"`
}

// walked is what an outing really covered: the walked track when there is
// one, else the route.
func walked(o *store.Outing) (dist, gain float64, minutes int) {
	var c geo.Comparison
	if len(o.Actual) > 0 && jsonUnmarshal(o.Actual, &c) == nil && c.DistanceM > 0 {
		m := int(c.TotalS / 60)
		if m == 0 {
			m = o.Minutes
		}
		return c.DistanceM, c.GainM, m
	}
	if st, e1 := time.Parse(time.RFC3339, o.StartedAt); e1 == nil {
		if en, e2 := time.Parse(time.RFC3339, o.EndedAt); e2 == nil && en.After(st) {
			return o.DistanceM, o.GainM, int(en.Sub(st).Minutes())
		}
	}
	return o.DistanceM, o.GainM, o.Minutes
}

func (s *Server) totals(ctx context.Context, uid int64, year int, pace int) (*yearTotals, error) {
	y := strconv.Itoa(year)
	outs, facts, err := s.store.ListOutings(ctx, uid, store.OutingFilter{From: y + "-01-01", To: y + "-12-31", Status: []string{store.OutingDone}})
	if err != nil {
		return nil, err
	}
	t := &yearTotals{Year: year, ByActivity: map[string]*activityTotal{}, ByMonth: make([]monthTotal, 12)}
	for i := range t.ByMonth {
		t.ByMonth[i].Month = i + 1
	}
	routeIDs := map[int64]bool{}
	var longest float64
	for i, o := range outs {
		decorateOuting(o, facts[i], pace)
		d, g, m := walked(o)
		t.Outings++
		t.DistanceM += d
		t.GainM += g
		t.Minutes += m
		a := t.ByActivity[o.Activity]
		if a == nil {
			a = &activityTotal{}
			t.ByActivity[o.Activity] = a
		}
		a.Outings++
		a.DistanceM += d
		a.GainM += g
		a.Minutes += m
		if mo, err := strconv.Atoi(o.Date[5:7]); err == nil && mo >= 1 && mo <= 12 {
			t.ByMonth[mo-1].Outings++
			t.ByMonth[mo-1].DistanceM += d
			t.ByMonth[mo-1].GainM += g
		}
		if d > longest {
			longest, t.Longest = d, o
		}
		if o.RouteID != nil {
			routeIDs[*o.RouteID] = true
		}
	}
	t.DistanceM, t.GainM = math.Round(t.DistanceM), math.Round(t.GainM)
	if len(routeIDs) > 0 {
		var ids []int64
		for id := range routeIDs {
			ids = append(ids, id)
		}
		rs, err := s.store.ListRoutes(ctx, uid, store.RouteFilter{IDs: ids})
		if err == nil {
			arch, _ := s.store.ListRoutes(ctx, uid, store.RouteFilter{IDs: ids, Archived: true})
			for _, r := range append(rs, arch...) {
				if r.MaxEle != nil && (t.Highest == nil || *r.MaxEle > *t.Highest) {
					v := *r.MaxEle
					t.Highest = &v
				}
			}
		}
		wps, _ := s.store.AllWaypoints(ctx, uid)
		seen := map[string]bool{}
		for _, w := range wps {
			if w.Kind == "summit" && routeIDs[w.RouteID] {
				k := fmt.Sprintf("%.3f,%.3f", w.Lat, w.Lng)
				if !seen[k] {
					seen[k] = true
					t.Summits++
				}
			}
		}
	}
	if t.Longest != nil {
		t.Longest.ActualTrack = nil
	}
	return t, nil
}

// handleSummary is the small picture: for the sidebar, Pocket Day and AI.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	t := today()
	up, ufacts, err := s.store.ListOutings(ctx, uid, store.OutingFilter{From: t, Status: []string{store.OutingPlanned, store.OutingOut}, Limit: 50})
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	out, ofacts, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{Status: []string{store.OutingOut}})
	for i, o := range up {
		decorateOuting(o, ufacts[i], st.PacePct)
	}
	for i, o := range out {
		decorateOuting(o, ofacts[i], st.PacePct)
	}
	review, _, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{From: store.AddDays(t, -14), To: t, Status: []string{store.OutingDone}})
	toRate := 0
	for _, o := range review {
		if o.Rating == 0 {
			toRate++
		}
	}
	routes, _ := s.store.ListRoutes(ctx, uid, store.RouteFilter{})
	year, _ := s.totals(ctx, uid, time.Now().Year(), st.PacePct)
	res := map[string]any{"planned": len(up), "out": out, "to_rate": toRate, "routes": len(routes), "year": year, "today": t}
	if len(up) > 0 {
		n := up[0]
		res["next"] = map[string]any{"id": n.ID, "title": outingTitle(n), "date": n.Date, "start_time": n.StartTime, "activity": n.Activity, "route_id": n.RouteID,
			"distance_m": n.DistanceM, "gain_m": n.GainM, "minutes": n.Minutes, "days": daysBetween(t, n.Date), "url": s.link(fmt.Sprintf("/outings/%d", n.ID))}
	}
	writeJSON(w, http.StatusOK, res)
}

// handleToday is the Today page: out now, the next outing with what to
// know, what is coming, how recent ones went and the year.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	t := today()
	out, _, err := s.store.ListOutings(ctx, uid, store.OutingFilter{Status: []string{store.OutingOut}})
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	up, ufacts, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{From: t, Status: []string{store.OutingPlanned}, Limit: 8})
	for i, o := range up {
		decorateOuting(o, ufacts[i], st.PacePct)
	}
	var next map[string]any
	nextList := up
	if len(out) > 0 {
		next, _ = s.outingPage(ctx, uid, out[0].ID)
	} else if len(up) > 0 && daysBetween(t, up[0].Date) <= 14 {
		next, _ = s.outingPage(ctx, uid, up[0].ID)
		nextList = up[1:]
	}
	recent, rfacts, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{To: t, Status: []string{store.OutingDone}, Limit: 6, Desc: true})
	for i, o := range recent {
		decorateOuting(o, rfacts[i], st.PacePct)
	}
	// overdue: planned for a day that passed and never marked
	stale, sfacts, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{From: store.AddDays(t, -30), To: store.AddDays(t, -1), Status: []string{store.OutingPlanned}, Desc: true})
	for i, o := range stale {
		decorateOuting(o, sfacts[i], st.PacePct)
	}
	routes, _ := s.store.ListRoutes(ctx, uid, store.RouteFilter{})
	var ideas []*store.Route
	for _, rt := range routes {
		if rt.Done == 0 && rt.Next == "" {
			decorateRoute(rt, st.PacePct)
			ideas = append(ideas, rt)
		}
	}
	sort.SliceStable(ideas, func(i, j int) bool { return ideas[i].Favourite && !ideas[j].Favourite })
	if len(ideas) > 4 {
		ideas = ideas[:4]
	}
	year, _ := s.totals(ctx, uid, time.Now().Year(), st.PacePct)
	writeJSON(w, http.StatusOK, map[string]any{"today": t, "current": next, "upcoming": nextList, "recent": recent, "unmarked": stale, "ideas": ideas, "year": year,
		"routes": len(routes)})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	year := queryInt(r, "year")
	if year < 1900 || year > 3000 {
		year = time.Now().Year()
	}
	t, err := s.totals(ctx, uid, year, st.PacePct)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	prev, _ := s.totals(ctx, uid, year-1, st.PacePct)
	years := []int{}
	rows, err := s.store.DB.QueryContext(ctx, `SELECT DISTINCT substr(date, 1, 4) FROM outings WHERE user_id = ? AND status = 'done' ORDER BY 1 DESC`, uid)
	if err == nil {
		for rows.Next() {
			var y string
			if rows.Scan(&y) == nil {
				if n, err := strconv.Atoi(y); err == nil {
					years = append(years, n)
				}
			}
		}
		rows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": t, "previous": prev, "years": years})
}

// handleDay is one day for Pocket Day's journal: outings planned and done.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d := chiParam(r, "date")
	if !validDate(d) {
		writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
		return
	}
	st := s.settingsOf(ctx, uid)
	outs, facts, err := s.store.ListOutings(ctx, uid, store.OutingFilter{From: d, To: d})
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	items := []map[string]any{}
	for i, o := range outs {
		if o.Status == store.OutingCancelled {
			continue
		}
		decorateOuting(o, facts[i], st.PacePct)
		dist, gain, mins := walked(o)
		line := fmt.Sprintf("%s · %s up", fmtDist(dist, st.Units), fmtHeight(gain, st.Units))
		if mins > 0 {
			line += " · " + fmtMinutes(mins)
		}
		if n := peopleNames(o.People); n != "" {
			line += " · with " + n
		}
		items = append(items, map[string]any{"id": o.ID, "title": outingTitle(o), "status": o.Status, "activity": o.Activity, "start_time": o.StartTime,
			"distance_m": dist, "gain_m": gain, "minutes": mins, "rating": o.Rating, "people": peopleNames(o.People), "text": line, "route": o.RouteName,
			"url": s.link(fmt.Sprintf("/outings/%d", o.ID))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": d, "outings": items, "count": len(items)})
}

// handleSearch finds routes, outings, gear and spots (for Pocket Search too).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res := []map[string]any{}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": res})
		return
	}
	st := s.settingsOf(ctx, uid)
	routes, _ := s.store.ListRoutes(ctx, uid, store.RouteFilter{Q: q})
	for _, rt := range routes {
		decorateRoute(rt, st.PacePct)
		res = append(res, map[string]any{"type": "route", "id": rt.ID, "title": rt.Name, "subtitle": fmt.Sprintf("%s · %s · %s up · %s", activityWords[rt.Activity], fmtDist(rt.DistanceM, st.Units), fmtHeight(rt.GainM, st.Units), fmtMinutes(rt.Minutes)),
			"path": fmt.Sprintf("/routes/%d", rt.ID), "url": s.link(fmt.Sprintf("/routes/%d", rt.ID))})
	}
	outs, facts, _ := s.store.ListOutings(ctx, uid, store.OutingFilter{Q: q, Desc: true, Limit: 30})
	for i, o := range outs {
		decorateOuting(o, facts[i], st.PacePct)
		res = append(res, map[string]any{"type": "outing", "id": o.ID, "title": outingTitle(o), "subtitle": niceDate(o.Date) + " · " + statusWords[o.Status],
			"date": o.Date, "path": fmt.Sprintf("/outings/%d", o.ID), "url": s.link(fmt.Sprintf("/outings/%d", o.ID))})
	}
	gear, _ := s.store.ListGear(ctx, uid, false, q)
	for _, g := range gear {
		res = append(res, map[string]any{"type": "gear", "id": g.ID, "title": g.Name, "subtitle": fmt.Sprintf("Gear · %d g", g.WeightG), "path": "/gear", "url": s.link("/gear")})
	}
	wps, _ := s.store.AllWaypoints(ctx, uid)
	lq := strings.ToLower(q)
	n := 0
	for _, wp := range wps {
		if n >= 10 || !strings.Contains(strings.ToLower(wp.Name+" "+wp.Note), lq) {
			continue
		}
		n++
		res = append(res, map[string]any{"type": "spot", "id": wp.ID, "title": firstNonEmpty(wp.Name, kindWords[wp.Kind]), "subtitle": kindWords[wp.Kind] + " on a route",
			"path": fmt.Sprintf("/routes/%d", wp.RouteID), "url": s.link(fmt.Sprintf("/routes/%d", wp.RouteID))})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": res})
}

var statusWords = map[string]string{"planned": "planned", "out": "out now", "done": "done", "cancelled": "called off"}
