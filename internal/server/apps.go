package server

import (
	"context"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// ---- trails in the other Pocket apps ----
//
// As the person (their own Core link or direct token): the walked track
// from Pocket Movement, the forecast from Pocket Weather (weather.go), the
// outing in Pocket Calendar, companions from Pocket Contacts, the workout
// in Pocket Health, the plan by email through Pocket Mail, gear from
// Pocket Inventory, places along a route in Pocket Places, the trip from
// Pocket Travel, the report in Pocket Notes and gear to buy in Pocket
// Tasks. None of it is needed to plan a walk: when an app is not there,
// the page greys the button and says what to do.

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

// ---- Pocket Movement: what you really walked ----

// movementTrack reads the recorded points of an outing's time from Pocket
// Movement: from setting off to arriving when those were marked, else the
// whole day; then cut to the stretch near the route.
func (s *Server) movementTrack(ctx context.Context, c *clients, o *store.Outing, r *store.Route) ([]store.Pt, error) {
	from, to := s.outingWindow(o)
	if from.IsZero() {
		return nil, &userErr{"the outing has no day"}
	}
	if from.After(time.Now()) {
		return nil, &userErr{"the outing has not started yet"}
	}
	var pts []store.Pt
	sib, err := c.conn(ctx, appMovement, scopeMovementPts)
	if err == nil {
		for page := 1; page <= 4; page++ {
			v, err := getJSONFrom(ctx, sib, "/api/v1/points", url.Values{"start_at": {strconv.FormatInt(from.Unix(), 10)}, "end_at": {strconv.FormatInt(to.Unix(), 10)},
				"order": {"asc"}, "per_page": {"10000"}, "page": {strconv.Itoa(page)}})
			if err != nil {
				return nil, fmt.Errorf("Pocket Movement: %w", err)
			}
			list := rows(v, "points")
			for _, p := range list {
				lat, _ := strconv.ParseFloat(str(p, "latitude"), 64)
				lng, _ := strconv.ParseFloat(str(p, "longitude"), 64)
				if lat == 0 && lng == 0 {
					continue
				}
				if b, ok := p["anomaly"].(bool); ok && b {
					continue
				}
				pt := store.Pt{Lat: lat, Lng: lng, T: int64(num(p, "timestamp"))}
				if a, ok := p["altitude"].(float64); ok && a != 0 {
					pt.Ele = &a
				}
				pts = append(pts, pt)
			}
			if len(list) < 10000 {
				break
			}
		}
	} else {
		// without the points, the day's tracks (no time per point)
		sib, err2 := c.conn(ctx, appMovement, scopeMovementRead)
		if err2 != nil {
			return nil, err
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/tracks", url.Values{"start_at": {strconv.FormatInt(from.Unix(), 10)}, "end_at": {strconv.FormatInt(to.Unix(), 10)}})
		if err != nil {
			return nil, fmt.Errorf("Pocket Movement: %w", err)
		}
		m, _ := v.(map[string]any)
		for _, f := range rows(m["features"]) {
			props := obj(f, "properties")
			st, _ := time.Parse(time.RFC3339, str(props, "start_at"))
			en, _ := time.Parse(time.RFC3339, str(props, "end_at"))
			var line []store.Pt
			for _, c := range anyList(obj(f, "geometry")["coordinates"]) {
				xy, _ := c.([]any)
				if len(xy) < 2 {
					continue
				}
				lng, _ := xy[0].(float64)
				lat, _ := xy[1].(float64)
				line = append(line, store.Pt{Lat: lat, Lng: lng})
			}
			// spread the times over the line by distance
			cum := geo.Cumulative(line)
			if n := len(cum); n > 1 && cum[n-1] > 0 && !st.IsZero() && en.After(st) {
				span := en.Sub(st).Seconds()
				for i := range line {
					line[i].T = st.Unix() + int64(span*cum[i]/cum[n-1])
				}
			}
			pts = append(pts, line...)
		}
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T < pts[j].T })
	if len(pts) < 2 {
		return nil, &userErr{"Pocket Movement has nothing recorded for that time"}
	}
	if r != nil && len(r.Track) >= 2 {
		near := geo.Trim(r.Track, pts, 300)
		if len(near) < 2 {
			return nil, &userErr{"what Pocket Movement recorded that day is nowhere near the route"}
		}
		pts = near
	}
	return geo.Thin(geo.Clean(pts), maxTrack), nil
}

// outingWindow is the stretch of time to look in: the marked start and end
// (with half an hour either side), else the whole day.
func (s *Server) outingWindow(o *store.Outing) (time.Time, time.Time) {
	day, err := time.ParseInLocation("2006-01-02", o.Date, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	from, to := day, day.AddDate(0, 0, 1).Add(-time.Second)
	if t, err := time.Parse(time.RFC3339, o.StartedAt); err == nil {
		from = t.Add(-30 * time.Minute)
	}
	if t, err := time.Parse(time.RFC3339, o.EndedAt); err == nil && t.After(from) {
		to = t.Add(30 * time.Minute)
	}
	if to.After(time.Now()) {
		to = time.Now()
	}
	return from, to
}

// compareOuting keeps a walked track on an outing with its comparison.
func (s *Server) compareOuting(ctx context.Context, uid int64, o *store.Outing, r *store.Route, pts []store.Pt, source string) error {
	var route []store.Pt
	if r != nil {
		route = r.Track
	}
	st := s.settingsOf(ctx, uid)
	cmp := geo.Compare(route, pts, o.Activity, st.PacePct)
	if r != nil {
		decorateRoute(r, st.PacePct)
		cmp.PlannedMin = r.Minutes
	}
	o.ActualTrack, o.ActualSource = pts, source
	o.Actual = jsonRaw(toJSONs(cmp))
	o.UserID = uid
	return s.store.SetOutingActual(ctx, o)
}

// handleMovementDay lists what Pocket Movement recorded on a day, to make
// a route from (the planner's "From a day you recorded").
func (s *Server) handleMovementDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	date := r.URL.Query().Get("date")
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
		return
	}
	sib, err := c.conn(ctx, appMovement, scopeMovementRead)
	if err != nil {
		s.userFail(w, err, "movement")
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/tracks", url.Values{"start_at": {strconv.FormatInt(day.Unix(), 10)}, "end_at": {strconv.FormatInt(day.AddDate(0, 0, 1).Unix()-1, 10)}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Movement: "+err.Error())
		return
	}
	m, _ := v.(map[string]any)
	out := []map[string]any{}
	for _, f := range rows(m["features"]) {
		p := obj(f, "properties")
		var line [][]float64
		for _, c := range anyList(obj(f, "geometry")["coordinates"]) {
			if xy, _ := c.([]any); len(xy) >= 2 {
				lng, _ := xy[0].(float64)
				lat, _ := xy[1].(float64)
				line = append(line, []float64{lat, lng})
			}
		}
		if len(line) < 2 {
			continue
		}
		pts := make([]store.Pt, len(line))
		for i, ll := range line {
			pts[i] = store.Pt{Lat: ll[0], Lng: ll[1]}
		}
		out = append(out, map[string]any{"id": idOf(p), "start_at": str(p, "start_at"), "end_at": str(p, "end_at"), "distance_m": num(p, "distance"),
			"duration_s": num(p, "duration"), "mode": str(p, "dominant_mode"), "gain_m": num(p, "elevation_gain"), "preview": geo.Preview(pts, 80)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": out, "open_url": linkTo(sib, "/day/"+date)})
}

// handleRouteFromMovement makes a route from a stretch Pocket Movement
// recorded (a walk you did without planning it).
func (s *Server) handleRouteFromMovement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c := s.clients(ctx, uid)
	var in struct {
		From     string `json:"start_at"`
		To       string `json:"end_at"`
		Name     string `json:"name"`
		Activity string `json:"activity"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	from, e1 := time.Parse(time.RFC3339, in.From)
	to, e2 := time.Parse(time.RFC3339, in.To)
	if e1 != nil || e2 != nil || !to.After(from) {
		writeErr(w, http.StatusBadRequest, "pick a recorded stretch")
		return
	}
	o := &store.Outing{Date: from.In(time.Local).Format("2006-01-02"), StartedAt: from.Format(time.RFC3339), EndedAt: to.Format(time.RFC3339)}
	pts, err := s.movementTrack(ctx, c, o, nil)
	if err != nil {
		s.userFail(w, err, "movement")
		return
	}
	var keep []store.Pt
	for _, p := range pts {
		if p.T == 0 || (p.T >= from.Unix() && p.T <= to.Unix()) {
			keep = append(keep, store.Pt{Lat: p.Lat, Lng: p.Lng, Ele: p.Ele})
		}
	}
	if len(keep) < 2 {
		writeErr(w, http.StatusBadRequest, "nothing was recorded in that stretch")
		return
	}
	st := c.st
	act := in.Activity
	if !oneOf(act, store.Activities) {
		act = firstNonEmpty(st.DefaultActivity, "hike")
	}
	rt := &store.Route{Name: firstNonEmpty(strings.TrimSpace(in.Name), activityWords[act]+" on "+niceDate(o.Date)), Activity: act, Track: keep, Source: "movement",
		SourceRef: from.UTC().Format(time.RFC3339) + "/" + to.UTC().Format(time.RFC3339)}
	if have, err := s.store.RouteBySource(ctx, uid, "movement", rt.SourceRef); err == nil {
		writeJSON(w, http.StatusOK, have)
		return
	}
	if !geo.HasEle(rt.Track) && s.online(st) {
		rt.Track = geo.Thin(rt.Track, 1500)
		_ = s.fillHeights(ctx, rt.Track)
	}
	prepRoute(rt)
	if err := s.store.CreateRoute(ctx, uid, rt); err != nil {
		s.fail(w, err, "create route")
		return
	}
	s.publish(ctx, uid, "trails.route_saved", "route:"+itoa(rt.ID), s.routeEvent(rt, st))
	writeJSON(w, http.StatusCreated, rt)
}

// ---- Pocket Calendar ----

// syncOutingEvent keeps an outing's event in Pocket Calendar: made, moved,
// or removed (called off, deleted, or the person stopped wanting it).
func (s *Server) syncOutingEvent(ctx context.Context, uid int64, o *store.Outing, remove bool) {
	c := s.clients(ctx, uid)
	if !remove && !c.st.CalendarSync && o.CalRef == "" {
		return
	}
	sib, err := optional(c.conn(ctx, appCalendar, scopeCalendarWrite))
	if err != nil || sib == nil {
		return
	}
	o.UserID = uid
	if remove || o.Status == store.OutingCancelled {
		if o.CalRef != "" {
			_ = sib.client.Do(ctx, http.MethodDelete, "/api/v1/events/"+url.PathEscape(o.CalRef), nil, nil, nil)
			o.CalRef = ""
			_ = s.store.SetOutingRefs(ctx, o)
		}
		return
	}
	var r *store.Route
	if o.RouteID != nil {
		r, _ = s.store.GetRoute(ctx, uid, *o.RouteID)
	}
	title := outingTitle(o)
	var desc []string
	if r != nil {
		decorateRoute(r, c.st.PacePct)
		desc = append(desc, fmt.Sprintf("%s · %s up · about %s (%s)", fmtDist(r.DistanceM, c.st.Units), fmtHeight(r.GainM, c.st.Units), fmtMinutes(r.Minutes), levelWords[r.Level]))
	}
	if n := peopleNames(o.People); n != "" {
		desc = append(desc, "With "+n)
	}
	if o.BackBy != "" {
		desc = append(desc, "Back by "+strings.Replace(o.BackBy[11:], "T", " ", 1))
	}
	if o.Notes != "" {
		desc = append(desc, o.Notes)
	}
	link := s.link(fmt.Sprintf("/outings/%d", o.ID))
	desc = append(desc, "Pocket Trails: "+link)
	body := map[string]any{"title": clip(title, 200), "description": strings.Join(desc, "\n\n"), "url": link, "tz": time.Local.String(), "categories": []string{"Outdoors"}}
	if r != nil && r.StartLat != nil {
		body["location"] = firstNonEmpty(r.StartName, fmt.Sprintf("%.5f, %.5f", *r.StartLat, *r.StartLng))
	}
	if o.StartTime != "" {
		start := startAt(o)
		end := backByTime(o)
		if end.IsZero() || !end.After(start) {
			end = start.Add(time.Duration(max(o.Minutes, 60)) * time.Minute)
			if r != nil {
				end = start.Add(time.Duration(float64(max(r.Minutes, 60))*1.2) * time.Minute)
			}
		}
		body["start"], body["end"] = start.Format("2006-01-02T15:04"), end.Format("2006-01-02T15:04")
	} else {
		body["start"], body["all_day"] = o.Date, true
	}
	if o.CalRef != "" {
		var out map[string]any
		if err := sib.client.Do(ctx, http.MethodPut, "/api/v1/events/"+url.PathEscape(o.CalRef), nil, body, &out); err == nil {
			return
		}
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		s.log.Debug("calendar: outing event", "outing", o.ID, "err", err)
		return
	}
	ref := idOf(res)
	if v, ok := res["event_id"].(float64); ok && v > 0 {
		ref = strconv.FormatInt(int64(v), 10)
	}
	o.CalRef = strings.SplitN(ref, ":", 2)[0]
	_ = s.store.SetOutingRefs(ctx, o)
}

func outingTitle(o *store.Outing) string {
	if o.Title != "" {
		return o.Title
	}
	if o.RouteName != "" {
		return activityWords[o.Activity] + ": " + o.RouteName
	}
	return activityWords[o.Activity]
}

// ---- Pocket Contacts ----

// handleContactSearch is the companion and contact picker.
func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}})
		return
	}
	sib, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"12"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": "Pocket Contacts: " + err.Error()})
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, "contacts") {
		if str(x, "kind") == "org" {
			continue
		}
		out = append(out, map[string]any{"id": idOf(x), "name": str(x, "fn"), "email": firstValue(x["emails"]), "phone": firstValue(x["phones"]),
			"initials": str(x, "initials"), "color": str(x, "color")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// firstValue is the first email or phone of a card (strings or {value}).
func firstValue(v any) string {
	arr, _ := v.([]any)
	for _, x := range arr {
		switch t := x.(type) {
		case string:
			if t != "" {
				return t
			}
		case map[string]any:
			if s := str(t, "value", "email", "number"); s != "" {
				return s
			}
		}
	}
	return ""
}

// ---- Pocket Health ----

// logHealth puts a finished outing in Pocket Health as a workout (once).
func (s *Server) logHealth(ctx context.Context, c *clients, o *store.Outing) (string, error) {
	if o.HealthRef != "" {
		return "", nil
	}
	sib, err := c.conn(ctx, appHealth, scopeHealthWrite)
	if err != nil {
		return "", err
	}
	start, end := time.Time{}, time.Time{}
	var dist, gain float64
	var cmp geo.Comparison
	if len(o.Actual) > 0 && jsonUnmarshal(o.Actual, &cmp) == nil && cmp.From > 0 && cmp.To > cmp.From {
		start, end, dist, gain = time.Unix(cmp.From, 0), time.Unix(cmp.To, 0), cmp.DistanceM, cmp.GainM
	} else {
		if t, err := time.Parse(time.RFC3339, o.StartedAt); err == nil {
			start = t
		} else {
			start = startAt(o)
		}
		if t, err := time.Parse(time.RFC3339, o.EndedAt); err == nil && t.After(start) {
			end = t
		} else if o.Minutes > 0 {
			end = start.Add(time.Duration(o.Minutes) * time.Minute)
		}
		dist, gain = o.DistanceM, o.GainM
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return "", &userErr{"Pocket Health needs a start and an end: mark when you set off and came back, or take the track from Pocket Movement"}
	}
	body := map[string]any{"kind": healthKind(o.Activity), "name": outingTitle(o), "start_at": start.Format(time.RFC3339), "end_at": end.Format(time.RFC3339),
		"distance_m": math.Round(dist), "elevation_m": math.Round(gain), "notes": clip(joinNonEmpty("\n\n", o.Conditions, "From Pocket Trails"), 2000)}
	res, err := postJSONTo(ctx, sib, "/api/v1/workouts", body)
	if err != nil {
		return "", fmt.Errorf("Pocket Health: %w", err)
	}
	o.HealthRef = idOf(res)
	_ = s.store.SetOutingRefs(ctx, o)
	return linkTo(sib, "/workouts"), nil
}

// ---- Pocket Notes ----

// saveNote writes how an outing went as a note.
func (s *Server) saveNote(ctx context.Context, c *clients, o *store.Outing, r *store.Route) (string, error) {
	sib, err := c.conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		return "", err
	}
	content := s.outingMarkdown(o, r, c.st) + "\n\n— From Pocket Trails: " + s.link(fmt.Sprintf("/outings/%d", o.ID))
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(outingTitle(o)+" — "+niceDate(o.Date), 200), "content": content})
	if err != nil {
		return "", fmt.Errorf("Pocket Notes: %w", err)
	}
	o.NoteRef = idOf(res)
	_ = s.store.SetOutingRefs(ctx, o)
	return linkTo(sib, "/note/"+o.NoteRef), nil
}

// outingMarkdown is an outing as Markdown (a note, the AI's brief).
func (s *Server) outingMarkdown(o *store.Outing, r *store.Route, st store.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**, %s", activityWords[o.Activity], niceDate(o.Date))
	if o.StartTime != "" {
		b.WriteString(" from " + o.StartTime)
	}
	b.WriteString("\n\n")
	if r != nil {
		fmt.Fprintf(&b, "- Route: %s — %s, %s up, about %s (%s)\n", r.Name, fmtDist(r.DistanceM, st.Units), fmtHeight(r.GainM, st.Units), fmtMinutes(r.Minutes), levelWords[r.Level])
	}
	if n := peopleNames(o.People); n != "" {
		b.WriteString("- With: " + n + "\n")
	}
	var cmp geo.Comparison
	if len(o.Actual) > 0 && jsonUnmarshal(o.Actual, &cmp) == nil && cmp.DistanceM > 0 {
		fmt.Fprintf(&b, "- Walked: %s, %s up, %s on the move (%s in all)\n", fmtDist(cmp.DistanceM, st.Units), fmtHeight(cmp.GainM, st.Units), fmtMinutes(int(cmp.MovingS/60)), fmtMinutes(int(cmp.TotalS/60)))
	}
	if o.Rating > 0 {
		b.WriteString("- Rating: " + strings.Repeat("★", o.Rating) + strings.Repeat("☆", 5-o.Rating) + "\n")
	}
	if o.Conditions != "" {
		b.WriteString("\n**Trail conditions:** " + o.Conditions + "\n")
	}
	if o.Report != "" {
		b.WriteString("\n" + o.Report + "\n")
	}
	if o.Notes != "" && o.Report == "" {
		b.WriteString("\n" + o.Notes + "\n")
	}
	return strings.TrimSpace(b.String())
}

// ---- Pocket Mail: the plan, and "not back yet" ----

func mailHTML(body string) string {
	var h strings.Builder
	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"></head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433;line-height:1.55">`)
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		h.WriteString(`<p style="margin:0 0 12px">` + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>") + `</p>`)
	}
	h.WriteString(`</body></html>`)
	return h.String()
}

// sendMail sends one email from the person's own mailbox.
func (s *Server) sendMail(ctx context.Context, c *clients, toName, toAddr, subject, body string) error {
	if !reEmail.MatchString(toAddr) {
		return &userErr{"that email address does not look right"}
	}
	if s.cfg.DemoData {
		s.log.Debug("demo: email not sent", "to", toAddr, "subject", subject)
		return nil
	}
	sib, err := c.conn(ctx, appMail, scopeMailSend)
	if err != nil {
		return err
	}
	msg := map[string]any{"from": c.st.MailFrom, "to": []map[string]string{{"name": toName, "addr": toAddr}}, "subject": clip(subject, 200), "body_text": body, "body_html": mailHTML(body)}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out map[string]any
	if err := sib.client.Do(ctx, http.MethodPost, "/api/v1/send", nil, msg, &out); err != nil {
		return fmt.Errorf("Pocket Mail: %w", err)
	}
	return nil
}

// planText is an outing's plan in plain words, for someone who is not
// coming (and cannot open Trails): where, when, which way, who, when back.
func (s *Server) planText(u *store.User, o *store.Outing, r *store.Route, st store.Settings, extra string) string {
	name := firstNonEmpty(u.Name, u.Email)
	var b strings.Builder
	fmt.Fprintf(&b, "Hello %s,\n\n", firstNonEmpty(o.ContactName, "there"))
	what := strings.ToLower(activityWords[o.Activity])
	fmt.Fprintf(&b, "%s is going on a %s on %s", name, what, niceDate(o.Date))
	if o.StartTime != "" {
		b.WriteString(", setting off at " + o.StartTime)
	}
	b.WriteString(".\n\n")
	if r != nil {
		fmt.Fprintf(&b, "Route: %s\n%s, %s up, about %s.\n", r.Name, fmtDist(r.DistanceM, st.Units), fmtHeight(r.GainM, st.Units), fmtMinutes(r.Minutes))
		if r.StartLat != nil {
			fmt.Fprintf(&b, "Start: %s%.5f, %.5f — https://www.openstreetmap.org/?mlat=%.5f&mlon=%.5f#map=15/%.5f/%.5f\n", prefixed(r.StartName), *r.StartLat, *r.StartLng, *r.StartLat, *r.StartLng, *r.StartLat, *r.StartLng)
		}
		if r.EndLat != nil && !r.Loop {
			fmt.Fprintf(&b, "End: %.5f, %.5f\n", *r.EndLat, *r.EndLng)
		}
		if r.MaxEle != nil {
			fmt.Fprintf(&b, "Highest point: %s\n", fmtHeight(*r.MaxEle, st.Units))
		}
		b.WriteString("\n")
	}
	if n := peopleNames(o.People); n != "" {
		b.WriteString("Going with: " + n + "\n\n")
	}
	if bt := backByTime(o); !bt.IsZero() {
		fmt.Fprintf(&b, "Plans to be back by %s on %s.\n\n", bt.Format("15:04"), niceDate(bt.Format("2006-01-02")))
	}
	if extra = strings.TrimSpace(extra); extra != "" {
		b.WriteString(extra + "\n\n")
	}
	return b.String()
}

func prefixed(s string) string {
	if s == "" {
		return ""
	}
	return s + ", "
}

// ---- Pocket Tasks: gear to buy ----

func (s *Server) gearTasks(ctx context.Context, c *clients, o *store.Outing) (int, string, error) {
	sib, err := c.conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return 0, "", err
	}
	due := store.AddDays(o.Date, -1)
	if due < today() {
		due = today()
	}
	n := 0
	for _, it := range o.Items {
		if !it.ToBuy || it.TaskRef != "" {
			continue
		}
		res, err := postJSONTo(ctx, sib, "/api/v1/tasks", map[string]any{"title": clip("Buy: "+it.Name, 200), "due": due,
			"notes": "For " + outingTitle(o) + " on " + niceDate(o.Date) + "\n\nFrom Pocket Trails: " + s.link(fmt.Sprintf("/outings/%d", o.ID)), "url": s.link(fmt.Sprintf("/outings/%d", o.ID))})
		if err != nil {
			return n, "", fmt.Errorf("Pocket Tasks: %w", err)
		}
		it.TaskRef = idOf(res)
		it.UserID = c.u.ID
		_ = s.store.UpdateOutingItem(ctx, it)
		n++
	}
	return n, linkTo(sib, "/"), nil
}

// ---- Pocket Places: along the way ----

// placesNear lists the person's saved places within 1.5 km of a route.
func (s *Server) placesNear(ctx context.Context, c *clients, r *store.Route) ([]map[string]any, string, error) {
	sib, err := c.conn(ctx, appPlaces, scopePlacesRead)
	if err != nil {
		return nil, "", err
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/places", url.Values{"limit": {"2000"}})
	if err != nil {
		return nil, "", fmt.Errorf("Pocket Places: %w", err)
	}
	thin := geo.Thin(r.Track, 400)
	out := []map[string]any{}
	for _, p := range rows(v, "places") {
		lat, lok := p["lat"].(float64)
		lng, gok := p["lng"].(float64)
		if !lok || !gok || len(thin) == 0 {
			continue
		}
		// a quick box test first
		if r.StartLat != nil && geo.Dist(lat, lng, *r.StartLat, *r.StartLng) > r.DistanceM+5000 {
			continue
		}
		at, off := geo.Along(thin, lat, lng)
		if off > 1500 {
			continue
		}
		out = append(out, map[string]any{"id": idOf(p), "name": str(p, "name"), "kind": str(p, "kind"), "status": str(p, "status"), "rating": p["rating"],
			"lat": lat, "lng": lng, "at_m": math.Round(at), "off_m": math.Round(off), "url": linkTo(sib, "/places/"+idOf(p))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["at_m"].(float64) < out[j]["at_m"].(float64) })
	return out, linkTo(sib, "/"), nil
}

func (s *Server) handleRoutePlaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rt, err := s.store.GetRoute(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "route")
		return
	}
	out, open, err := s.placesNear(ctx, s.clients(ctx, uid), rt)
	if err != nil {
		if isNotConnected(err) {
			writeJSON(w, http.StatusOK, map[string]any{"places": []any{}, "off": err.Error()})
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": out, "open_url": open})
}

// placeKind is the Pocket Places kind of a spot.
func placeKind(k string) string {
	switch k {
	case "viewpoint", "photo":
		return "viewpoint"
	case "hut", "camp", "shelter":
		return "hotel"
	case "food":
		return "restaurant"
	case "lake":
		return "beach"
	}
	return "nature"
}

// handleWaypointToPlace saves a spot on a route as a place in Pocket Places.
func (s *Server) handleWaypointToPlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wp, err := s.store.GetWaypoint(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "spot")
		return
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appPlaces, scopePlacesWrite)
	if err != nil {
		s.userFail(w, err, "places")
		return
	}
	if wp.PlaceRef != "" {
		writeJSON(w, http.StatusOK, map[string]any{"waypoint": wp, "url": linkTo(sib, "/places/"+wp.PlaceRef)})
		return
	}
	rt, _ := s.store.GetRoute(ctx, uid, wp.RouteID)
	notes := wp.Note
	if rt != nil {
		notes = joinNonEmpty("\n\n", wp.Note, "On "+rt.Name+" (Pocket Trails)")
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/places", map[string]any{"name": firstNonEmpty(wp.Name, kindWords[wp.Kind]), "kind": placeKind(wp.Kind), "status": "want",
		"lat": wp.Lat, "lng": wp.Lng, "notes": notes, "source_ref": fmt.Sprintf("trails:wp:%d", wp.ID)})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Places: "+err.Error())
		return
	}
	wp.PlaceRef = idOf(res)
	_ = s.store.UpdateWaypoint(ctx, wp)
	writeJSON(w, http.StatusOK, map[string]any{"waypoint": wp, "url": linkTo(sib, "/places/"+wp.PlaceRef)})
}

// ---- Pocket Travel and Pocket Inventory: pickers ----

// handleAppOptions lists what to pick from in an app: trips, inventory items.
func (s *Server) handleAppOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	switch chiParam(r, "what") {
	case "trips":
		sib, err := c.conn(ctx, appTravel, scopeTripsRead)
		if err != nil {
			s.userFail(w, err, "trips")
			return
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/trips", nil)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Pocket Travel: "+err.Error())
			return
		}
		date := r.URL.Query().Get("date")
		out := []map[string]any{}
		for _, t := range rows(v, "trips") {
			sd, ed := day10(t["start_date"]), day10(t["end_date"])
			during := date != "" && sd != "" && sd <= date && (ed == "" || date <= ed)
			out = append(out, map[string]any{"id": idOf(t), "name": str(t, "name"), "destination": str(t, "destination"), "start_date": sd, "end_date": ed,
				"during": during, "url": linkTo(sib, "/trips/"+idOf(t))})
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i]["during"] != out[j]["during"] {
				return out[i]["during"].(bool)
			}
			return out[i]["start_date"].(string) > out[j]["start_date"].(string)
		})
		writeJSON(w, http.StatusOK, out)
	case "inventory":
		sib, err := c.conn(ctx, appInventory, scopeItemsRead)
		if err != nil {
			s.userFail(w, err, "inventory")
			return
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/items", url.Values{"q": {r.URL.Query().Get("q")}})
		if err != nil {
			writeErr(w, http.StatusBadGateway, "Pocket Inventory: "+err.Error())
			return
		}
		have := map[string]bool{}
		if gear, err := s.store.ListGear(ctx, c.u.ID, false, ""); err == nil {
			for _, g := range gear {
				if g.InventoryRef != "" {
					have[g.InventoryRef] = true
				}
			}
		}
		out := []map[string]any{}
		for _, it := range rows(v, "items") {
			if b, ok := it["archived"].(bool); ok && b {
				continue
			}
			id := idOf(it)
			out = append(out, map[string]any{"id": id, "name": str(it, "name"), "brand": str(it, "brand"), "category": str(it, "category_name"), "have": have[id]})
			if len(out) >= 300 {
				break
			}
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// handleApps says per app whether Trails can reach it, and if not why and
// what to do — the pages grey out what does not work.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// ---- keeping up ----

// syncAll looks, for everyone, for the walked tracks of outings finished
// in the last two days that have none yet (Pocket Movement uploads late).
func (s *Server) syncAll(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		st := s.settingsOf(ctx, uid)
		if !st.MovementAuto || s.cfg.DemoData {
			continue
		}
		outs, _, err := s.store.ListOutings(ctx, uid, store.OutingFilter{From: store.AddDays(today(), -2), To: today(), Status: []string{store.OutingDone}})
		if err != nil {
			continue
		}
		c := s.clients(ctx, uid)
		for _, o := range outs {
			if o.ActualSource != "" || !s.once(ctx, fmt.Sprintf("movement:%d:%d:%s", uid, o.ID, time.Now().Format("2006010215"))) {
				continue
			}
			s.takeTrack(ctx, c, o.ID)
		}
	}
}

// takeTrack fetches and keeps an outing's walked track from Movement, then
// logs it in Health when the person wants that. Quiet on failure.
func (s *Server) takeTrack(ctx context.Context, c *clients, id int64) {
	o, rf, err := s.store.GetOuting(ctx, c.u.ID, id)
	if err != nil {
		return
	}
	decorateOuting(o, rf, c.st.PacePct)
	var r *store.Route
	if o.RouteID != nil {
		r, _ = s.store.GetRoute(ctx, c.u.ID, *o.RouteID)
	}
	if o.ActualSource == "" {
		if st := c.status(ctx, appMovement); st.Via == "" {
			return
		}
		pts, err := s.movementTrack(ctx, c, o, r)
		if err != nil {
			s.log.Debug("movement: track", "outing", id, "err", err)
		} else if err := s.compareOuting(ctx, c.u.ID, o, r, pts, "movement"); err != nil {
			s.log.Warn("movement: keep track", "outing", id, "err", err)
		}
	}
	if c.st.HealthLog && o.HealthRef == "" {
		if st := c.status(ctx, appHealth); st.Via != "" {
			if _, err := s.logHealth(ctx, c, o); err != nil {
				s.log.Debug("health: workout", "outing", id, "err", err)
			}
		}
	}
}

func anyList(v any) []any {
	a, _ := v.([]any)
	return a
}
