package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Trails (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what /search, /day and the Today page read, as the request's
// person, and change nothing: no outing is set off, finished or called off
// from here (finishing one can email the person's safety contact), no
// forecast is fetched (the one kept on the outing is shown) and no track
// leaves the app.

// gearWords are the gear groups as the Gear page names them.
var gearWords = map[string]string{"pack": "Packs", "shelter": "Shelter", "sleep": "Sleep", "clothing": "Clothing", "footwear": "Footwear", "cook": "Cooking",
	"water": "Water", "food": "Food", "navigation": "Navigation", "safety": "Safety", "first_aid": "First aid", "electronics": "Electronics", "bike": "Bike",
	"snow": "Snow and ice", "other": ""}

// wentWords start the line of a finished outing on a route: "Hiked Trebević".
var wentWords = map[string]string{"hike": "Hiked", "walk": "Walked", "run": "Ran", "bike": "Rode", "gravel": "Rode", "mtb": "Rode", "ski": "Skied"}

// hasWord reports whether there is a letter or a digit to look for.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// dots joins the parts of a line that are there: "Hike · 5.4 km · 468 m up".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// tripNumbers is how far, how much up and how long, in the person's units,
// as the lists show them: "5.4 km · 468 m up · 2 h 30". What is not known
// is left out.
func tripNumbers(dist, gain float64, minutes int, units string) string {
	var d, g, m string
	if dist > 0 {
		d = fmtDist(dist, units)
	}
	if gain >= 1 {
		g = fmtHeight(gain, units) + " up"
	}
	if minutes > 0 {
		m = fmtMinutes(minutes)
	}
	return dots(d, g, m)
}

// fmtWeight is a weight as the Gear page shows it: "310 g", "1.25 kg", "10.9 oz".
func fmtWeight(g int, units string) string {
	if units == "imperial" {
		oz := float64(g) / 28.3495
		if oz >= 16 {
			return fmt.Sprintf("%.1f lb", oz/16)
		}
		return fmt.Sprintf("%.1f oz", oz)
	}
	switch {
	case g >= 10000:
		return fmt.Sprintf("%.1f kg", float64(g)/1000)
	case g >= 1000:
		return fmt.Sprintf("%.2f kg", float64(g)/1000)
	}
	return fmt.Sprintf("%d g", g)
}

// fmtTemp is a temperature in the person's units: "12°", "54°F".
func fmtTemp(t float64, units string) string {
	if units == "imperial" {
		return fmt.Sprintf("%d°F", int(math.Round(t*9/5+32)))
	}
	return fmt.Sprintf("%d°", int(math.Round(t)))
}

// clock is a time of day ("08:00") the way the person reads it (Settings → time format).
func clock(hhmm, format string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	if format == "12h" {
		return t.Format("3:04 PM")
	}
	return t.Format("15:04")
}

// excerpt is a short piece of a longer text around the words looked for.
func excerpt(text, q string) string {
	const width = 140
	text = strings.Join(strings.Fields(text), " ")
	rs := []rune(text)
	if len(rs) <= width {
		return text
	}
	from := 0
	low := strings.ToLower(text)
	if i := strings.Index(low, strings.ToLower(q)); i > 0 {
		from = utf8.RuneCountInString(low[:i]) - 40
	}
	from = max(0, min(from, len(rs)-width))
	out := strings.TrimSpace(string(rs[from : from+width]))
	if from > 0 {
		out = "…" + out
	}
	if from+width < len(rs) {
		out += "…"
	}
	return out
}

// foundIn says where the words were found when the line itself does not
// show it: the first of the details (label, text) that has them.
func foundIn(q string, details ...[2]string) string {
	lq := strings.ToLower(q)
	for _, d := range details {
		if !strings.Contains(strings.ToLower(d[1]), lq) {
			continue
		}
		if d[0] == "" {
			return excerpt(d[1], q)
		}
		return d[0] + ": " + excerpt(d[1], q)
	}
	return ""
}

// with is who came along: "with Amar, Lejla".
func with(ps []store.Person) string {
	if n := peopleNames(ps); n != "" {
		return "with " + n
	}
	return ""
}

// shortName is an outing in a word or two for a line that says the rest:
// its own title, else its route, else what it is ("Hike").
func shortName(o *store.Outing) string {
	return firstNonEmpty(strings.TrimSpace(o.Title), o.RouteName, activityWords[o.Activity], "Outing")
}

// routeNames are the names of a person's routes with these ids, archived
// ones too.
func (s *Server) routeNames(ctx context.Context, uid int64, ids []int64) map[int64]string {
	names := map[int64]string{}
	if len(ids) == 0 {
		return names
	}
	args := []any{uid}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.store.DB.QueryContext(ctx, `SELECT id, name FROM routes WHERE user_id = ? AND id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			names[id] = name
		}
	}
	return names
}

// ---------- search ----------

// pocketSearch answers Pocket Search: the routes, outings, gear and spots
// /search finds, as one list. What has the words in its name comes first,
// then what has them in its details; inside each, a route, an outing (the
// newest first), a piece of gear and a spot take turns. Words without a
// letter or a digit find nothing.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	f, err := s.find(ctx, uid, q, st.PacePct)
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	has := func(v ...string) bool { return strings.Contains(strings.ToLower(strings.Join(v, " ")), lq) }
	score := func(title string) float64 {
		if has(title) {
			return 2
		}
		return 1
	}
	out := []core.SearchItem{}
	for _, rt := range f.routes {
		it := core.SearchItem{ID: "route:" + itoa(rt.ID), Title: rt.Name, Kind: "route", URL: "/routes/" + itoa(rt.ID), Score: score(rt.Name),
			Snippet: dots(activityWords[rt.Activity], tripNumbers(rt.DistanceM, rt.GainM, rt.Minutes, st.Units), rt.Region)}
		if !has(it.Title, it.Snippet) {
			it.Snippet = dots(it.Snippet, foundIn(q, [2]string{"Starts at", rt.StartName}, [2]string{"Tags", strings.Join(rt.Tags, ", ")}, [2]string{"", rt.Description}))
		}
		out = append(out, it)
	}
	for _, o := range f.outings {
		dist, gain, mins := walked(o)
		it := core.SearchItem{ID: "outing:" + itoa(o.ID), Title: outingTitle(o), Kind: "outing", Date: o.Date, URL: "/outings/" + itoa(o.ID),
			Snippet: dots(upperFirst(statusWords[o.Status]), tripNumbers(dist, gain, mins, st.Units), with(o.People))}
		it.Score = score(it.Title)
		if !has(it.Title, it.Snippet) {
			it.Snippet = dots(it.Snippet, foundIn(q, [2]string{"Route", o.RouteName}, [2]string{"Conditions", o.Conditions}, [2]string{"", o.Report}, [2]string{"", o.Notes}))
		}
		out = append(out, it)
	}
	for _, g := range f.gear {
		it := core.SearchItem{ID: "gear:" + itoa(g.ID), Title: g.Name, Kind: "gear", URL: "/gear", Score: score(g.Name)}
		var weight, worn, used string
		if g.WeightG > 0 {
			weight = fmtWeight(g.WeightG, st.Units)
		}
		if g.Worn {
			worn = "worn"
		}
		if g.Consumable {
			used = "used up"
		}
		it.Snippet = dots(gearWords[g.Category], weight, worn, used, excerpt(g.Note, q))
		out = append(out, it)
	}
	var on []int64
	for _, wp := range f.spots {
		on = append(on, wp.RouteID)
	}
	routes := s.routeNames(ctx, uid, on)
	for _, wp := range f.spots {
		it := core.SearchItem{ID: "spot:" + itoa(wp.ID), Title: firstNonEmpty(wp.Name, kindWords[wp.Kind]), Kind: "spot", URL: "/routes/" + itoa(wp.RouteID)}
		it.Score = score(it.Title)
		where, after := kindWords[wp.Kind]+" on "+firstNonEmpty(routes[wp.RouteID], "a route"), ""
		if wp.AtM > 0 {
			after = "after " + fmtDist(wp.AtM, st.Units)
		}
		it.Snippet = dots(where, after, excerpt(wp.Note, q))
		out = append(out, it)
	}
	out = inTurn(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// inTurn orders matches for a short list: the ones with the words in their
// name before the rest, and inside each a route, an outing, a piece of gear
// and a spot in turn, so the many outings on one route do not push its
// spots off the list.
func inTurn(items []core.SearchItem) []core.SearchItem {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	out := make([]core.SearchItem, 0, len(items))
	for from := 0; from < len(items); {
		to := from
		for to < len(items) && items[to].Score == items[from].Score {
			to++
		}
		var kinds []string
		by := map[string][]core.SearchItem{}
		for _, it := range items[from:to] {
			if _, ok := by[it.Kind]; !ok {
				kinds = append(kinds, it.Kind)
			}
			by[it.Kind] = append(by[it.Kind], it)
		}
		for left := to - from; left > 0; {
			for _, k := range kinds {
				if len(by[k]) > 0 {
					out, by[k] = append(out, by[k][0]), by[k][1:]
					left--
				}
			}
		}
		from = to
	}
	return out
}

// ---------- day ----------

// clockOn is the time of day of a moment when it falls on that day in the
// zone, else "".
func clockOn(moment, date string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339, moment)
	if err != nil {
		return ""
	}
	if t = t.In(loc); t.Format("2006-01-02") != date {
		return ""
	}
	return t.Format("15:04")
}

// dayLine is one outing in the day's line: "Hiked Trebević, 12.4 km in 4 h"
// for one that is done, "Hike: Trebević, out now" on the way, "Hike:
// Trebević planned for 08:00" before.
func dayLine(o *store.Outing, dist float64, minutes int, st store.Settings) string {
	switch o.Status {
	case store.OutingDone:
		line := strings.TrimSpace(o.Title)
		switch {
		case line != "":
		case o.RouteName != "" && wentWords[o.Activity] != "":
			line = wentWords[o.Activity] + " " + o.RouteName
		case o.RouteName != "":
			line = outingTitle(o)
		case o.Activity == "other" || activityWords[o.Activity] == "":
			line = "Went on an outing"
		default:
			line = "Went for a " + strings.ToLower(activityWords[o.Activity])
		}
		switch {
		case dist > 0 && minutes > 0:
			line += ", " + fmtDist(dist, st.Units) + " in " + fmtMinutes(minutes)
		case dist > 0:
			line += ", " + fmtDist(dist, st.Units)
		case minutes > 0:
			line += ", " + fmtMinutes(minutes)
		}
		return line
	case store.OutingOut:
		return outingTitle(o) + ", out now"
	}
	if o.StartTime != "" {
		return outingTitle(o) + " planned for " + clock(o.StartTime, st.TimeFormat)
	}
	return outingTitle(o) + " planned"
}

// pocketDay answers Pocket Day: the outings of that day as /day lists them
// (the ones called off left out), one item each, with what was really
// walked when a track says so. An outing's day is a plain date; the zone
// only places the moments of setting off and coming back on the clock.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	outs, err := s.dayOutings(ctx, uid, date, st.PacePct)
	if err != nil {
		return nil, err
	}
	if len(outs) == 0 {
		return nil, nil
	}
	if tz == nil {
		tz = time.Local
	}
	day := &core.Day{Date: date}
	var done, out, planned []string
	var distSum, gainSum float64
	var minSum int
	for _, o := range outs {
		dist, gain, mins := walked(o)
		it := core.DayItem{ID: itoa(o.ID), Title: outingTitle(o), Kind: statusWords[o.Status], Done: o.Status == store.OutingDone, Time: o.StartTime,
			Minutes: float64(mins), Qty: math.Round(dist/100) / 10, Note: clip(firstNonEmpty(strings.TrimSpace(o.Report), strings.TrimSpace(o.Notes)), 500),
			URL: "/outings/" + itoa(o.ID)}
		route := o.RouteName
		if strings.Contains(strings.ToLower(it.Title), strings.ToLower(route)) {
			route = "" // "Hike: Trebević" says it already
		}
		it.Sub = dots(route, with(o.People))
		// the real times, once the person set off and came back
		if o.Status != store.OutingPlanned {
			if t := clockOn(o.StartedAt, date, tz); t != "" {
				it.Time = t
			}
		}
		if o.Status == store.OutingDone && it.Time != "" {
			if t := clockOn(o.EndedAt, date, tz); t > it.Time {
				it.End = t
			}
		}
		day.Items = append(day.Items, it)
		line := dayLine(o, dist, mins, st)
		switch o.Status {
		case store.OutingDone:
			done = append(done, line)
			distSum, gainSum, minSum = distSum+dist, gainSum+gain, minSum+mins
		case store.OutingOut:
			out = append(out, line)
		default:
			planned = append(planned, line)
		}
	}
	// what was done first, then what is under way, then what is planned
	lines := append(append(done, out...), planned...)
	more := len(lines) - 3
	if more > 0 {
		lines = lines[:3]
	}
	day.Summary = strings.Join(lines, "; ")
	if more > 0 {
		day.Summary += fmt.Sprintf(" and %d more", more)
	}
	if distSum > 0 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Distance", Value: fmtDist(distSum, st.Units)})
	}
	if gainSum >= 1 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Climb", Value: fmtHeight(gainSum, st.Units)})
	}
	if minSum > 0 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Time", Value: fmtMinutes(minSum)})
	}
	return day, nil
}

// ---------- widget ----------

// dayName is a day close to today in a word: "today", "tomorrow",
// "Saturday" within the week, else "Sat 12 Dec".
func dayName(from, date string) string {
	switch d := daysBetween(from, date); {
	case d == 0:
		return "today"
	case d == 1:
		return "tomorrow"
	case d > 1 && d < 7:
		if t, err := store.ParseDate(date); err == nil {
			return t.Format("Monday")
		}
	}
	return niceDate(date)
}

// weatherWords is the forecast kept on an outing for its day, in a few
// words and the person's units: "Partly cloudy, 4° – 12°, 40% chance of
// rain". Nothing is fetched; without a kept forecast for that day it is "".
func weatherWords(o *store.Outing, units string) string {
	var w dayWeather
	if len(o.Weather) == 0 || json.Unmarshal(o.Weather, &w) != nil || w.Date != o.Date {
		return ""
	}
	parts := []string{firstNonEmpty(w.Condition, w.Text, wmoWords(w.Code))}
	switch {
	case w.TMin != nil && w.TMax != nil:
		parts = append(parts, fmtTemp(*w.TMin, units)+" – "+fmtTemp(*w.TMax, units))
	case w.TMax != nil:
		parts = append(parts, fmtTemp(*w.TMax, units))
	}
	if w.PrecipProb != nil && *w.PrecipProb >= 10 {
		what := "rain"
		if (w.Code >= 71 && w.Code <= 77) || w.Code == 85 || w.Code == 86 {
			what = "snow"
		}
		parts = append(parts, fmt.Sprintf("%.0f%% chance of %s", *w.PrecipProb, what))
	}
	return strings.Join(parts, ", ")
}

// backByWords is an outing's back-by time: "back by 17:30" (with the day
// when it is not today), "was due back by 17:30" once it has passed.
func backByWords(o *store.Outing, st store.Settings, now time.Time) string {
	t := backByTime(o)
	if t.IsZero() {
		return ""
	}
	when := clock(t.Format("15:04"), st.TimeFormat)
	if day := t.Format("2006-01-02"); day != now.Format("2006-01-02") {
		when = niceDate(day) + " " + when
	}
	if now.After(t) {
		return "was due back by " + when
	}
	return "back by " + when
}

// pocketWidget answers Pocket Hub with what the Today page leads with: the
// outing the person is out on with its back-by time, the next planned ones
// with their day, start time and the weather kept for them, then the latest
// finished ones. It reads a few rows and asks nobody: the forecast is the
// one already kept on the outing. Without any outing there is nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	t, now := today(), time.Now()
	list := func(f store.OutingFilter) ([]*store.Outing, error) {
		outs, facts, err := s.store.ListOutings(ctx, uid, f)
		for i, o := range outs {
			decorateOuting(o, facts[i], st.PacePct)
		}
		return outs, err
	}
	out, err := list(store.OutingFilter{Status: []string{store.OutingOut}, Desc: true, Limit: 2})
	if err != nil {
		return nil, err
	}
	next, err := list(store.OutingFilter{From: t, Status: []string{store.OutingPlanned}, Limit: 3})
	if err != nil {
		return nil, err
	}
	recent, err := list(store.OutingFilter{To: t, Status: []string{store.OutingDone}, Desc: true, Limit: 3})
	if err != nil {
		return nil, err
	}
	if len(out)+len(next)+len(recent) == 0 {
		return nil, nil
	}
	w := &core.WidgetData{}
	row := func(o *store.Outing, detail string, done bool) {
		w.Items = append(w.Items, core.WidgetItem{ID: itoa(o.ID), Title: outingTitle(o), Detail: detail, Done: done, URL: "/outings/" + itoa(o.ID)})
	}
	for _, o := range out {
		row(o, dots("Out now", backByWords(o, st, now)), false)
	}
	for _, o := range next {
		when := upperFirst(dayName(t, o.Date))
		if o.StartTime != "" {
			when += " " + clock(o.StartTime, st.TimeFormat)
		}
		row(o, dots(when, tripNumbers(o.DistanceM, o.GainM, o.Minutes, st.Units), weatherWords(o, st.Units)), false)
	}
	for _, o := range recent {
		dist, gain, mins := walked(o)
		row(o, dots(niceDate(o.Date), tripNumbers(dist, gain, mins, st.Units)), true)
	}
	switch {
	case len(out) > 0:
		w.Line = "Out now: " + shortName(out[0])
		if by := backByWords(out[0], st, now); by != "" {
			w.Line += ", " + by
		}
	case len(next) > 0:
		o := next[0]
		when := dayName(t, o.Date)
		if d := daysBetween(t, o.Date); d > 1 {
			when = "on " + when
		}
		if o.StartTime != "" {
			when += " " + clock(o.StartTime, st.TimeFormat)
		}
		w.Line = shortName(o) + " " + when
	}
	return w, nil
}
