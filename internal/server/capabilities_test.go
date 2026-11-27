package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/config"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write> from Pocket Hub
// and remembers the manifest Trails registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "trails:read", "write": "trails:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "trails", "scope": scopes[parts[2]],
				"user": map[string]any{"id": 1, "sub": parts[1] + "@example.com", "email": parts[1] + "@example.com", "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// trail brings the test route in for a person, with looking things up
// online switched off: no test asks the internet for a forecast.
func (a *app) trail(tok string) map[string]any {
	a.t.Helper()
	a.ok(tok, "PUT", "/api/v1/settings", map[string]any{"online": false})
	return importGPX(a, tok)
}

// plan adds an outing and returns it.
func (a *app) plan(tok string, body map[string]any) map[string]any {
	a.t.Helper()
	return a.expect(201, tok, "POST", "/api/v1/outings", body)["outing"].(map[string]any)
}

// numbersOf is a route's or an outing's line of numbers, as the lists show it.
func numbersOf(m map[string]any, units string) string {
	return tripNumbers(num(m, "distance_m"), num(m, "gain_m"), int(num(m, "minutes")), units)
}

// statusOf reads an outing's status straight from the store.
func (a *app) statusOf(oid string) string {
	a.t.Helper()
	var st string
	if err := a.st.DB.QueryRow(`SELECT status FROM outings WHERE id = ?`, n(oid)).Scan(&st); err != nil {
		a.t.Fatal(err)
	}
	return st
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	rt := a.trail(a.alice)
	rid := id(rt)
	o := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": today(), "start_time": "08:00", "people": []map[string]any{{"name": "Amar"}}})
	g := a.expect(201, a.alice, "POST", "/api/v1/gear", map[string]any{"name": "Head torch", "category": "electronics", "weight_g": 80, "note": "spare batteries in the lid"})

	// a name that has the words: the route, then the outing on it
	all := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe", nil))
	if len(all) != 2 {
		t.Fatalf("trebe: %v", all)
	}
	if r := all[0]; r["id"] != "route:"+rid || r["kind"] != "route" || r["title"] != "Trebević loop" || r["url"] != "/routes/"+rid || r["date"] != nil ||
		r["snippet"] != "Hike · "+numbersOf(rt, "metric") || !regexp.MustCompile(`^Hike · \d\.\d km · \d+ m up · \d h( \d\d)?$`).MatchString(r["snippet"].(string)) {
		t.Fatalf("route: %v", r)
	}
	if r := all[1]; r["id"] != "outing:"+id(o) || r["kind"] != "outing" || r["title"] != "Hike: Trebević loop" || r["url"] != "/outings/"+id(o) || r["date"] != today() ||
		r["snippet"] != "Planned · "+numbersOf(rt, "metric")+" · with Amar" {
		t.Fatalf("outing: %v", r)
	}
	// the spots along a route, with the route they are on
	spot := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=drinking", nil), "Drinking water")
	if spot["kind"] != "spot" || !strings.HasPrefix(spot["id"].(string), "spot:") || spot["url"] != "/routes/"+rid || !strings.HasPrefix(spot["snippet"].(string), "Water on Trebević loop") {
		t.Fatalf("spot: %v", spot)
	}
	// gear, in the person's units
	gear := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=torch", nil), "Head torch")
	if gear["id"] != "gear:"+id(g) || gear["kind"] != "gear" || gear["url"] != "/gear" || gear["snippet"] != "Electronics · 80 g · spare batteries in the lid" {
		t.Fatalf("gear: %v", gear)
	}
	// words found in the notes are shown; an outing found by its route says so
	named := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(today(), 2), "title": "Morning round", "notes": "Bring the crampons, the top is icy."})
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=crampons", nil)); len(got) != 1 || got[0]["id"] != "outing:"+id(named) ||
		got[0]["snippet"] != "Planned · "+numbersOf(rt, "metric")+" · Bring the crampons, the top is icy." {
		t.Fatalf("by notes: %v", got)
	}
	got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe", nil))
	if len(got) != 3 || got[2]["title"] != "Morning round" || !strings.HasSuffix(got[2]["snippet"].(string), " · Route: Trebević loop") {
		t.Fatalf("by route name, after the names that match: %v", got)
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe&limit=1", nil)); len(got) != 1 || got[0]["kind"] != "route" {
		t.Fatalf("limit 1: %v", got)
	}
	// miles and feet for someone who reads those
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"units": "imperial"})
	if r := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe", nil))[0]; r["snippet"] != "Hike · "+numbersOf(rt, "imperial") || !strings.Contains(r["snippet"].(string), " mi · ") ||
		!strings.Contains(r["snippet"].(string), " ft up") {
		t.Fatalf("imperial: %v", r)
	}
	if r := item(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=torch", nil), "Head torch"); r["snippet"] != "Electronics · 2.8 oz · spare batteries in the lid" {
		t.Fatalf("imperial gear: %v", r)
	}
	// nothing to look for finds nothing, not everything
	for _, q := range []string{"", "??", "*", "%25", "_", "+"} {
		if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("%q: %v", q, got)
		}
	}
	// no track, no line of a route, no coordinates
	_, raw := a.raw(a.alice, "GET", "/api/v1/pocket/search?q=e&limit=50", "", nil)
	for _, no := range []string{"track", "preview", "lat", "43.8", "18.4"} {
		if strings.Contains(string(raw), no) {
			t.Fatalf("the answer carries %q: %s", no, raw)
		}
	}
	// another person's routes, outings, spots and gear never appear
	for _, q := range []string{"trebe", "hike", "drinking", "torch", "crampons", "amar", "e"} {
		if got := items(t, a.ok(a.bob, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("bob finds alice's %q: %v", q, got)
		}
	}
	a.expect(401, "", "GET", "/api/v1/pocket/search?q=trebe", nil)
	// the old route still answers as before
	old := a.ok(a.alice, "GET", "/api/v1/search?q=Trebe", nil)["results"].([]any)
	if first := old[0].(map[string]any); len(old) != 3 || first["type"] != "route" || first["path"] != "/routes/"+rid || first["url"] != "http://trails.test/routes/"+rid ||
		!strings.HasPrefix(first["subtitle"].(string), "Hike · ") {
		t.Fatalf("/search changed: %v", old)
	}
	// a short list has one of each before a second of any: the outings on a route do not hide its spots
	a.expect(201, a.alice, "POST", "/api/v1/routes/"+rid+"/waypoints", map[string]any{"kind": "hut", "name": "Trebević hut", "lat": 43.8372, "lng": 18.4561})
	a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(today(), 9)})
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe&limit=4", nil)); len(got) != 4 || got[0]["kind"] != "route" || got[1]["kind"] != "outing" ||
		got[2]["kind"] != "spot" || got[2]["title"] != "Trebević hut" || !strings.HasPrefix(got[2]["snippet"].(string), "Hut on Trebević loop · after ") || got[3]["kind"] != "outing" {
		t.Fatalf("in turn: %v", got)
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	rt := a.trail(a.alice)
	rid, day := id(rt), today()
	km := fmtDist(num(rt, "distance_m"), "metric")
	o := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": day, "start_time": "08:00", "people": []map[string]any{{"name": "Amar"}}, "notes": "Meet at the cable car."})
	oid := id(o)
	off := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": day, "title": "Evening round"})
	a.ok(a.alice, "POST", "/api/v1/outings/"+id(off)+"/status/cancel", nil)
	a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(day, 1), "title": "Tomorrow's round"})

	// planned: one item, with the route's numbers
	d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil)
	if d["date"] != day || d["summary"] != "Hike: Trebević loop planned for 08:00" || d["metrics"] != nil || len(items(t, d)) != 1 {
		t.Fatalf("a planned day: %v", d)
	}
	it := items(t, d)[0]
	if it["id"] != oid || it["title"] != "Hike: Trebević loop" || it["kind"] != "planned" || it["time"] != "08:00" || it["end"] != nil || it["done"] != nil ||
		it["sub"] != "with Amar" || it["note"] != "Meet at the cable car." || it["url"] != "/outings/"+oid ||
		num(it, "minutes") != num(rt, "minutes") || num(it, "qty") != float64(int(num(rt, "distance_m")/100+0.5))/10 {
		t.Fatalf("planned: %v", it)
	}
	// on the way
	a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/status/start", nil)
	d = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil)
	if it := items(t, d)[0]; d["summary"] != "Hike: Trebević loop, out now" || it["kind"] != "out now" || it["done"] != nil ||
		!regexp.MustCompile(`^\d\d:\d\d$`).MatchString(it["time"].(string)) {
		t.Fatalf("out: %v", d)
	}
	// back: what was done, with the real times
	a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/status/done", nil)
	a.ok(a.alice, "PUT", "/api/v1/outings/"+oid, map[string]any{"report": "Clear all the way, snow from the bobsleigh track up."})
	at := func(hm string) string {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hm, time.Local)
		return tm.UTC().Format(time.RFC3339)
	}
	if _, err := a.st.DB.Exec(`UPDATE outings SET started_at = ?, ended_at = ? WHERE id = ?`, at("08:10"), at("12:40"), n(oid)); err != nil {
		t.Fatal(err)
	}
	// and a run without a route or a name
	run := a.plan(a.alice, map[string]any{"date": day, "activity": "run"})
	a.ok(a.alice, "POST", "/api/v1/outings/"+id(run)+"/status/done", nil)
	later := a.plan(a.alice, map[string]any{"date": day, "title": "Evening walk", "activity": "walk"})
	d = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day+"?tz=Europe/Sarajevo", nil)
	if d["summary"] != "Went for a run; Hiked Trebević loop, "+km+" in 4 h 30; Evening walk planned" || len(items(t, d)) != 3 {
		t.Fatalf("a day out: %v", d)
	}
	a.ok(a.alice, "DELETE", "/api/v1/outings/"+id(later), nil)
	d = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil)
	it = item(t, d, "Hike: Trebević loop")
	if it["kind"] != "done" || it["done"] != true || it["time"] != "08:10" || it["end"] != "12:40" || num(it, "minutes") != 270 ||
		it["note"] != "Clear all the way, snow from the bobsleigh track up." || it["sub"] != "with Amar" {
		t.Fatalf("done: %v", it)
	}
	if it := item(t, d, "Run"); it["kind"] != "done" || it["qty"] != nil || it["minutes"] != nil || it["time"] != nil || it["sub"] != nil {
		t.Fatalf("a run without numbers: %v", it)
	}
	ms := d["metrics"].([]any)
	for i, want := range [][2]string{{"Distance", km}, {"Climb", fmtHeight(num(rt, "gain_m"), "metric")}, {"Time", "4 h 30"}} {
		if m := ms[i].(map[string]any); len(ms) != 3 || m["label"] != want[0] || m["value"] != want[1] {
			t.Fatalf("metrics: %v", ms)
		}
	}
	// an outing with its own name keeps it, and names its route beside it
	a.ok(a.alice, "PUT", "/api/v1/outings/"+oid, map[string]any{"title": "Birthday hike"})
	d = a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil)
	if it := item(t, d, "Birthday hike"); d["summary"] != "Went for a run; Birthday hike, "+km+" in 4 h 30" || it["sub"] != "Trebević loop · with Amar" {
		t.Fatalf("a named outing: %v", d)
	}
	// the person's own units and clock
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"units": "imperial", "time_format": "12h"})
	a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(day, 3), "start_time": "14:30"})
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+store.AddDays(day, 3), nil); d["summary"] != "Hike: Trebević loop planned for 2:30 PM" || items(t, d)[0]["time"] != "14:30" {
		t.Fatalf("12-hour clock: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil); d["summary"] != "Went for a run; Birthday hike, "+fmtDist(num(rt, "distance_m"), "imperial")+" in 4 h 30" ||
		!strings.HasSuffix(d["metrics"].([]any)[0].(map[string]any)["value"].(string), " mi") {
		t.Fatalf("imperial: %v", d)
	}
	// a day with nothing, and somebody else's day
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2019-03-04", nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("an empty day: %v", d)
	}
	if d := a.ok(a.bob, "GET", "/api/v1/pocket/day/"+day, nil); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("bob sees alice's day: %v", d)
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/yesterday", nil)
	a.expect(401, "", "GET", "/api/v1/pocket/day/"+day, nil)
	// the old route still answers as before
	if old := a.ok(a.alice, "GET", "/api/v1/day/"+day, nil); num(old, "count") != 2 || old["outings"].([]any)[0].(map[string]any)["status"] != "done" {
		t.Fatalf("/day changed: %v", old)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	uid, day := a.ids["Alice"], today()
	// no outings: nothing to show
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || w["metrics"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("without outings: %v", w)
	}
	rt := a.trail(a.alice)
	rid := id(rt)
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("a route alone is no outing: %v", w)
	}
	// the next outing, with the forecast Trails already keeps for its day
	in3 := store.AddDays(day, 3)
	weekday := func(d string) string { tm, _ := store.ParseDate(d); return tm.Format("Monday") }
	next := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": in3, "start_time": "08:00"})
	kept := `{"date":"` + in3 + `","code":2,"condition":"Partly cloudy","t_min":4,"t_max":12,"precip_prob":40,"source":"weather","fetched_at":"2026-01-01T00:00:00Z"}`
	if err := a.st.SetOutingWeather(context.Background(), uid, n(id(next)), kept); err != nil {
		t.Fatal(err)
	}
	later := a.plan(a.alice, map[string]any{"date": store.AddDays(day, 20), "title": "Ski weekend", "activity": "ski"})
	did := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(day, -1)})
	a.ok(a.alice, "POST", "/api/v1/outings/"+id(did)+"/status/done", nil)
	off := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": store.AddDays(day, 1), "title": "Called off"})
	a.ok(a.alice, "POST", "/api/v1/outings/"+id(off)+"/status/cancel", nil)

	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	rows := items(t, w)
	if w["line"] != "Trebević loop on "+weekday(in3)+" 08:00" || w["metrics"] != nil || len(rows) != 3 {
		t.Fatalf("widget: %v", w)
	}
	if r := rows[0]; r["id"] != id(next) || r["title"] != "Hike: Trebević loop" || r["url"] != "/outings/"+id(next) || r["done"] != nil ||
		r["detail"] != weekday(in3)+" 08:00 · "+numbersOf(rt, "metric")+" · Partly cloudy, 4° – 12°, 40% chance of rain" {
		t.Fatalf("the next outing: %v", r)
	}
	if r := rows[1]; r["title"] != "Ski weekend" || r["detail"] != niceDate(store.AddDays(day, 20)) || r["id"] != id(later) {
		t.Fatalf("a later one, no forecast made up: %v", r)
	}
	if r := rows[2]; r["id"] != id(did) || r["done"] != true || r["detail"] != niceDate(store.AddDays(day, -1))+" · "+numbersOf(rt, "metric") {
		t.Fatalf("the latest one done: %v", r)
	}
	// the widget read the forecast, it did not fetch one
	var now string
	if err := a.st.DB.QueryRow(`SELECT weather FROM outings WHERE id = ?`, n(id(next))).Scan(&now); err != nil || now != kept {
		t.Fatalf("the kept forecast changed: %v %s", err, now)
	}
	// today and tomorrow in words, the person's clock and units
	a.ok(a.alice, "PUT", "/api/v1/outings/"+id(next), map[string]any{"date": store.AddDays(day, 1)})
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != "Trebević loop tomorrow 08:00" || !strings.HasPrefix(items(t, w)[0]["detail"].(string), "Tomorrow 08:00 · ") {
		t.Fatalf("tomorrow: %v", w)
	}
	a.ok(a.alice, "PUT", "/api/v1/outings/"+id(next), map[string]any{"date": in3})
	_ = a.st.SetOutingWeather(context.Background(), uid, n(id(next)), kept)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"units": "imperial", "time_format": "12h"})
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != "Trebević loop on "+weekday(in3)+" 8:00 AM" ||
		items(t, w)[0]["detail"] != weekday(in3)+" 8:00 AM · "+numbersOf(rt, "imperial")+" · Partly cloudy, 39°F – 54°F, 40% chance of rain" {
		t.Fatalf("12-hour clock and miles: %v", w)
	}
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"units": "metric", "time_format": "24h"})
	// out now: the back-by time leads
	tomorrow := store.AddDays(day, 1)
	out := a.plan(a.alice, map[string]any{"route_id": n(rid), "date": day, "start_time": "07:00", "back_by": tomorrow + "T10:00"})
	a.ok(a.alice, "POST", "/api/v1/outings/"+id(out)+"/status/start", nil)
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	rows = items(t, w)
	if w["line"] != "Out now: Trebević loop, back by "+niceDate(tomorrow)+" 10:00" || len(rows) != 4 || rows[0]["id"] != id(out) ||
		rows[0]["detail"] != "Out now · back by "+niceDate(tomorrow)+" 10:00" || rows[1]["id"] != id(next) || rows[3]["id"] != id(did) {
		t.Fatalf("out now: %v", w)
	}
	yesterday := store.AddDays(day, -1)
	a.ok(a.alice, "PUT", "/api/v1/outings/"+id(out), map[string]any{"back_by": yesterday + "T10:00"})
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != "Out now: Trebević loop, was due back by "+niceDate(yesterday)+" 10:00" {
		t.Fatalf("overdue: %v", w)
	}
	// reading changed nothing: nobody was marked back, nothing was planned again
	a.ok(a.alice, "GET", "/api/v1/pocket/day/"+day, nil)
	a.ok(a.alice, "GET", "/api/v1/pocket/search?q=trebe", nil)
	for oid, want := range map[string]string{id(out): "out", id(next): "planned", id(did): "done", id(off): "cancelled"} {
		if got := a.statusOf(oid); got != want {
			t.Fatalf("outing %s is %s, want %s", oid, got, want)
		}
	}
	// still nothing of this for Bob
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("bob sees alice's outings: %v", w)
	}
	a.expect(401, "", "GET", "/api/v1/pocket/widget", nil)
}

func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	rt := a.trail(a.alice)
	day := today()
	o := a.plan(a.alice, map[string]any{"route_id": n(id(rt)), "date": day, "start_time": "08:00"})
	oid := id(o)
	a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/status/start", nil)
	line := map[string]any{"request_id": "req-0000000000000001", "text": "Trebević on Saturday"}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=trebe", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/" + day, nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/" + day, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=trebe", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=trebe", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/" + day, nil, 403},
		// Trails keeps no quick lines: there is no such route, for anybody
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line, 404},
		{"pcx_alice_widget", "POST", "/api/v1/pocket/capture", line, 404},
		{a.alice, "POST", "/api/v1/pocket/capture", line, 404},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/auth/me", nil, 200},
		// the rest of the app, and above all an outing's status
		{"pcx_alice_widget", "POST", "/api/v1/outings/" + oid + "/status/back", nil, 403},
		{"pcx_alice_day", "POST", "/api/v1/outings/" + oid + "/status/done", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/outings/" + oid + "/status/cancel", nil, 403},
		{"pcx_alice_widget", "POST", "/api/v1/outings/" + oid + "/send-plan", nil, 403},
		{"pcx_alice_widget", "POST", "/api/v1/outings/" + oid + "/weather", nil, 403},
		{"pcx_alice_widget", "PUT", "/api/v1/outings/" + oid, map[string]any{"title": "x"}, 403},
		{"pcx_alice_widget", "GET", "/api/v1/today", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/outings", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/search?q=trebe", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/routes", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/routes/" + id(rt) + "/gpx", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/" + day, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/outings/" + oid, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/settings", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/export.json", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=trebe", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/" + day, nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/search?q=trebe", nil, 200},
		{"pcx_alice_read", "GET", "/api/v1/day/" + day, nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=trebe", nil)); len(got) != 2 || got[0]["title"] != "Trebević loop" {
		t.Fatalf("alice through the link: %v", got)
	}
	if w := a.ok("pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil); w["line"] != "Out now: Trebević loop" || len(items(t, w)) != 1 {
		t.Fatalf("alice's widget through the link: %v", w)
	}
	if d := a.ok("pcx_alice_day", "GET", "/api/v1/pocket/day/"+day, nil); len(items(t, d)) != 1 || d["summary"] != "Hike: Trebević loop, out now" {
		t.Fatalf("alice's day through the link: %v", d)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=trebe", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's route: %v", got)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if d := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/"+day, nil); len(items(t, d)) != 0 {
		t.Fatalf("bob's day through the link: %v", d)
	}
	// after all of it she is still out: nobody marked her back
	if got := a.statusOf(oid); got != "out" {
		t.Fatalf("the outing is %s", got)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeWidget, req("GET", "/api/v1/today"), false},
		{core.ScopeWidget, req("POST", "/api/v1/pocket/widget"), false},
		{core.ScopeWidget, req("POST", "/api/v1/outings/3/status/back"), false},
		{core.ScopeDay, req("POST", "/api/v1/outings"), false},
		{"trails:read", req("GET", "/api/v1/pocket/search?q=x"), false},
		{"trails:read", req("GET", "/api/v1/pocket/widget"), false},
		{"trails:write", req("GET", "/api/v1/pocket/day/2026-01-02"), false},
		{"trails:write", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Trails tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "trails" || m["category"] != "Places" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || day == nil || day["label"] != "Trails" || day["one"] != "outing" || day["many"] != "outings" || widget == nil || widget["title"] != "Next outing" {
		t.Fatalf("capabilities: %v %v %v", search, day, widget)
	}
	if m["capture"] != nil {
		t.Fatalf("Trails keeps no quick lines: %v", m["capture"])
	}
	bangs, kinds := search["bangs"].([]any), search["kinds"].([]any)
	if len(bangs) != 3 || bangs[0] != "trails" || bangs[1] != "hike" || bangs[2] != "route" || len(kinds) != 4 {
		t.Fatalf("search: %v", search)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"trails:read", "trails:write", core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeMoveExport, core.ScopeMoveImport} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
	if offered[core.ScopeCapture] {
		t.Error("pocket:capture is offered")
	}
}
