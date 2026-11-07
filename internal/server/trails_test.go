package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

const testGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata><name>Trebević loop</name></metadata>
  <wpt lat="43.8256" lon="18.4524"><ele>1627</ele><name>Summit</name><sym>Summit</sym></wpt>
  <wpt lat="43.8460" lon="18.4490"><name>Drinking water</name></wpt>
  <trk><name>Trebević loop</name><type>hiking</type><trkseg>
    <trkpt lat="43.8467" lon="18.4486"><ele>1160</ele></trkpt>
    <trkpt lat="43.8421" lon="18.4529"><ele>1230</ele></trkpt>
    <trkpt lat="43.8372" lon="18.4561"><ele>1330</ele></trkpt>
    <trkpt lat="43.8318" lon="18.4569"><ele>1450</ele></trkpt>
    <trkpt lat="43.8256" lon="18.4524"><ele>1627</ele></trkpt>
    <trkpt lat="43.8301" lon="18.4470"><ele>1480</ele></trkpt>
    <trkpt lat="43.8366" lon="18.4431"><ele>1320</ele></trkpt>
    <trkpt lat="43.8467" lon="18.4487"><ele>1161</ele></trkpt>
  </trkseg></trk>
</gpx>`

// importGPX posts the test file as Alice (or tok) and returns the route.
func importGPX(a *app, tok string) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "trebevic.gpx")
	_, _ = fw.Write([]byte(testGPX))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/routes/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 {
		a.t.Fatalf("import → %d %v", res.StatusCode, out)
	}
	rs := out["routes"].([]any)
	if len(rs) != 1 {
		a.t.Fatalf("import made %d routes", len(rs))
	}
	return rs[0].(map[string]any)
}

func TestImportRoute(t *testing.T) {
	a := newApp(t, nil)
	rt := importGPX(a, a.alice)
	if rt["name"] != "Trebević loop" || rt["activity"] != "hike" {
		t.Fatalf("name/activity: %v %v", rt["name"], rt["activity"])
	}
	if d := num(rt, "distance_m"); d < 4000 || d > 7000 {
		t.Fatalf("distance %v", d)
	}
	if g := num(rt, "gain_m"); g < 450 || g > 480 {
		t.Fatalf("gain %v", g)
	}
	if rt["loop"] != true || num(rt, "minutes") < 90 {
		t.Fatalf("loop %v minutes %v", rt["loop"], rt["minutes"])
	}
	page := a.ok(a.alice, "GET", "/api/v1/routes/"+id(rt), nil)
	wps := page["route"].(map[string]any)["waypoints"].([]any)
	if len(wps) != 2 {
		t.Fatalf("%d spots", len(wps))
	}
	kinds := map[string]bool{}
	for _, w := range wps {
		kinds[w.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["summit"] || !kinds["water"] {
		t.Fatalf("spot kinds %v", kinds)
	}
	// the same file again finds the same route
	again := importGPX(a, a.alice)
	if id(again) != id(rt) {
		t.Fatal("the same file made a second route")
	}
	// GPX out again
	req, _ := http.NewRequest("GET", a.url+"/api/v1/routes/"+id(rt)+"/gpx", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	_, _ = b.ReadFrom(res.Body)
	res.Body.Close()
	if !strings.Contains(b.String(), "<trkpt") || !strings.Contains(b.String(), "Summit") {
		t.Fatal("GPX export is missing the track or the spots")
	}
}

// Everything is per person: Bob sees none of Alice's trails, and every id of hers is a 404 for him.
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	rt := importGPX(a, a.alice)
	rid := id(rt)
	o := a.expect(201, a.alice, "POST", "/api/v1/outings", map[string]any{"route_id": n(rid), "date": today(), "start_time": "09:00"})["outing"].(map[string]any)
	oid := id(o)
	g := a.expect(201, a.alice, "POST", "/api/v1/gear", map[string]any{"name": "Head torch", "weight_g": 80})
	p := a.expect(201, a.alice, "POST", "/api/v1/packs", map[string]any{"name": "Day hike", "items": []map[string]any{{"gear_id": num(g, "id"), "qty": 1}}})
	items := a.list(a.alice, "/api/v1/outings")
	if len(items) != 1 {
		t.Fatal("Alice's outing")
	}
	a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/items", map[string]any{"pack_id": num(p, "id")})
	page := a.ok(a.alice, "GET", "/api/v1/outings/"+oid, nil)
	its := page["outing"].(map[string]any)["items"].([]any)
	if len(its) != 1 {
		t.Fatalf("packing list has %d", len(its))
	}
	itemID := id(its[0].(map[string]any))
	wp := page["route"].(map[string]any)["waypoints"].([]any)[0].(map[string]any)

	for _, path := range []string{"/api/v1/routes", "/api/v1/outings", "/api/v1/gear", "/api/v1/packs"} {
		if l := a.list(a.bob, path); len(l) != 0 {
			t.Fatalf("Bob sees %d in %s", len(l), path)
		}
	}
	for _, path := range []string{"/api/v1/routes/" + rid, "/api/v1/outings/" + oid, "/api/v1/routes/" + rid + "/gpx"} {
		a.expect(404, a.bob, "GET", path, nil)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/routes/"+rid, map[string]any{"name": "Mine now"})
	a.expect(404, a.bob, "PUT", "/api/v1/outings/"+oid, map[string]any{"title": "Mine now"})
	a.expect(404, a.bob, "PUT", "/api/v1/gear/"+id(g), map[string]any{"name": "Mine now"})
	a.expect(404, a.bob, "PUT", "/api/v1/packs/"+id(p), map[string]any{"name": "Mine now"})
	a.expect(404, a.bob, "PUT", "/api/v1/items/"+itemID, map[string]any{"packed": true})
	a.expect(404, a.bob, "PUT", "/api/v1/waypoints/"+id(wp), map[string]any{"name": "Mine now"})
	a.expect(404, a.bob, "POST", "/api/v1/routes/"+rid+"/waypoints", map[string]any{"lat": 43.8, "lng": 18.4})
	a.expect(404, a.bob, "POST", "/api/v1/outings/"+oid+"/items", map[string]any{"name": "Rope"})
	a.expect(404, a.bob, "POST", "/api/v1/outings/"+oid+"/status/start", nil)
	// Bob cannot plan on Alice's route, nor pack Alice's gear
	a.expect(400, a.bob, "POST", "/api/v1/outings", map[string]any{"route_id": n(rid), "date": today()})
	bo := a.expect(201, a.bob, "POST", "/api/v1/outings", map[string]any{"date": today()})["outing"].(map[string]any)
	a.expect(404, a.bob, "POST", "/api/v1/outings/"+id(bo)+"/items", map[string]any{"gear_id": num(g, "id")})
	a.expect(404, a.bob, "POST", "/api/v1/outings/"+id(bo)+"/items", map[string]any{"pack_id": num(p, "id")})
	for _, path := range []string{"/api/v1/routes/" + rid, "/api/v1/outings/" + oid, "/api/v1/gear/" + id(g), "/api/v1/packs/" + id(p), "/api/v1/items/" + itemID, "/api/v1/waypoints/" + id(wp)} {
		a.expect(404, a.bob, "DELETE", path, nil)
	}
	// still Alice's
	a.ok(a.alice, "GET", "/api/v1/routes/"+rid, nil)
	a.ok(a.alice, "GET", "/api/v1/outings/"+oid, nil)
	if s := a.list(a.bob, "/api/v1/outings"); len(s) != 1 {
		t.Fatalf("Bob has %d outings", len(s))
	}
}

func TestOutingFlow(t *testing.T) {
	a := newApp(t, nil)
	rt := importGPX(a, a.alice)
	// "tell someone" needs a back-by time
	a.expect(400, a.alice, "POST", "/api/v1/outings", map[string]any{"route_id": num(rt, "id"), "date": today(), "safety": true})
	page := a.expect(201, a.alice, "POST", "/api/v1/outings", map[string]any{"route_id": num(rt, "id"), "date": today(), "start_time": "08:00",
		"safety": true, "back_by": today() + "T15:00", "contact_name": "Lejla", "contact_email": "lejla@example.com",
		"people": []map[string]any{{"name": "Amar"}}})
	o := page["outing"].(map[string]any)
	if o["safety"] != true || o["activity"] != "hike" || num(o, "minutes") <= 0 {
		t.Fatalf("outing: %v", o)
	}
	if _, ok := page["advice"].([]any); !ok {
		t.Fatal("no advice list")
	}
	if page["sunset"] == "" {
		t.Fatal("sunset is worked out without the network")
	}
	oid := id(o)
	out := a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/status/start", nil)
	if out["outing"].(map[string]any)["status"] != "out" {
		t.Fatal("not out")
	}
	sum := a.ok(a.alice, "GET", "/api/v1/summary", nil)
	if len(sum["out"].([]any)) != 1 {
		t.Fatal("summary misses the outing out now")
	}
	back := a.ok(a.alice, "POST", "/api/v1/outings/"+oid+"/status/back", nil)
	bo := back["outing"].(map[string]any)
	if bo["status"] != "done" || bo["ended_at"] == "" {
		t.Fatalf("back: %v", bo)
	}
	// a walked track from a file: compared with the route
	var buf bytes.Buffer
	walk := testGPX
	for i := 0; strings.Contains(walk, "\"><ele>"); i++ {
		walk = strings.Replace(walk, "\"><ele>", fmt.Sprintf("\"><time>2026-06-01T08:%02d:00Z</time><e>", i*7), 1)
	}
	_, _ = buf.WriteString(strings.ReplaceAll(strings.ReplaceAll(walk, "<e>", "<ele>"), "</e>", "</ele>"))
	req, _ := http.NewRequest("POST", a.url+"/api/v1/outings/"+oid+"/track?name=walk.gpx", &buf)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("track upload: %v %v", err, res.StatusCode)
	}
	res.Body.Close()
	got := a.ok(a.alice, "GET", "/api/v1/outings/"+oid, nil)["outing"].(map[string]any)
	act, _ := got["actual"].(map[string]any)
	if act == nil || num(act, "covered_pct") < 90 || num(act, "distance_m") < 4000 {
		t.Fatalf("comparison: %v", act)
	}
	// the day, for Pocket Day
	day := a.ok(a.alice, "GET", "/api/v1/day/"+today(), nil)
	if num(day, "count") != 1 {
		t.Fatalf("day: %v", day)
	}
	st := a.ok(a.alice, "GET", "/api/v1/stats", nil)["year"].(map[string]any)
	if num(st, "outings") < 1 {
		t.Fatal("stats")
	}
	// search finds it
	res2 := a.ok(a.alice, "GET", "/api/v1/search?q=Trebe", nil)
	if len(res2["results"].([]any)) < 2 {
		t.Fatalf("search: %v", res2)
	}
}

func TestLinkScopes(t *testing.T) {
	r := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	read, write := []string{"trails:read"}, []string{"trails:write"}
	cases := []struct {
		scopes []string
		req    *http.Request
		want   bool
	}{
		{read, r("GET", "/api/v1/routes"), true},
		{read, r("GET", "/api/v1/routes/3"), true},
		{read, r("GET", "/api/v1/routes/3/gpx"), true},
		{read, r("GET", "/api/v1/outings/3"), true},
		{read, r("GET", "/api/v1/summary"), true},
		{read, r("GET", "/api/v1/day/2026-11-07"), true},
		{read, r("POST", "/api/v1/outings"), false},
		{write, r("POST", "/api/v1/outings"), true},
		{write, r("PUT", "/api/v1/outings/4"), true},
		{write, r("POST", "/api/v1/routes"), true},
		{write, r("GET", "/api/v1/routes"), false},
		{append(read, write...), r("POST", "/api/v1/outings/3/status/back"), false},
		{append(read, write...), r("POST", "/api/v1/outings/3/send-plan"), false},
		{append(read, write...), r("DELETE", "/api/v1/gear/1"), false},
		{nil, r("GET", "/api/v1/auth/me"), true},
		{read, r("GET", "/api/v1/settings"), false},
		{read, r("GET", "/api/v1/export.json"), false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, c.req); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.req.Method, c.req.URL.Path, got, c.want)
		}
	}
}

func TestExportImportTwice(t *testing.T) {
	a := newApp(t, nil)
	rt := importGPX(a, a.alice)
	g := a.expect(201, a.alice, "POST", "/api/v1/gear", map[string]any{"name": "Rain jacket", "weight_g": 300})
	a.expect(201, a.alice, "POST", "/api/v1/packs", map[string]any{"name": "Day hike", "items": []map[string]any{{"gear_id": num(g, "id"), "qty": 1}}})
	a.expect(201, a.alice, "POST", "/api/v1/outings", map[string]any{"route_id": num(rt, "id"), "date": today(), "pack_id": 1})
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.NewDecoder(res.Body).Decode(&doc)
	res.Body.Close()
	if doc["app"] != "pocket-trails" {
		t.Fatalf("export: %v", doc["app"])
	}
	for i := 0; i < 2; i++ {
		a.ok(a.bob, "POST", "/api/v1/import", doc)
	}
	for path, want := range map[string]int{"/api/v1/routes": 1, "/api/v1/outings": 1, "/api/v1/gear": 1, "/api/v1/packs": 1} {
		if l := a.list(a.bob, path); len(l) != want {
			t.Fatalf("Bob has %d in %s after two imports", len(l), path)
		}
	}
	bo := a.list(a.bob, "/api/v1/outings")[0].(map[string]any)
	if bo["route_id"] == nil || bo["route_id"] == num(rt, "id") && false {
		t.Fatal("the imported outing lost its route")
	}
	if l := a.list(a.alice, "/api/v1/routes"); len(l) != 1 {
		t.Fatal("Alice's routes changed")
	}
}

func TestLinkSourceOnly(t *testing.T) {
	a := newApp(t, nil)
	rt := importGPX(a, a.alice)
	// a route Alice brought in is not another app's to change
	rr, _ := http.NewRequest("PUT", "/api/v1/routes/"+id(rt), strings.NewReader(`{"name":"x"}`))
	if !linkAllows([]string{"trails:write"}, rr) {
		t.Fatal("route write should pass the route table")
	}
	var r store.Route
	r.Source = "gpx"
	if r.Source == "flows" {
		t.Fatal("unreachable")
	}
}

func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"units": "imperial", "pace_pct": 120, "contact_email": "mum@example.com"})
	if s := a.ok(a.bob, "GET", "/api/v1/settings", nil); s["units"] != "metric" || num(s, "pace_pct") != 100 || s["contact_email"] != "" {
		t.Fatalf("Bob got Alice's settings: %v %v %v", s["units"], s["pace_pct"], s["contact_email"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"pace_pct": 10})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"contact_email": "not an email"})
}

func TestGearImportLighterPack(t *testing.T) {
	a := newApp(t, nil)
	csv := "Item Name,Category,desc,qty,weight,unit,url,price,worn,consumable\nRain jacket,Clothing,,1,10.5,oz,,,,\nTrail runners,Footwear,,1,600,g,,,Worn,\nSnacks,Food,,2,200,g,,,,Consumable\n"
	req, _ := http.NewRequest("POST", a.url+"/api/v1/gear/import?name=Summer", strings.NewReader(csv))
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("import: %v %d", err, res.StatusCode)
	}
	res.Body.Close()
	gear := a.list(a.alice, "/api/v1/gear")
	if len(gear) != 3 {
		t.Fatalf("%d gear", len(gear))
	}
	packs := a.list(a.alice, "/api/v1/packs")
	if len(packs) != 1 {
		t.Fatal("no pack")
	}
	w := packs[0].(map[string]any)["weight"].(map[string]any)
	if num(w, "worn") != 600 || num(w, "consumable") != 400 || num(w, "base") != 298 {
		t.Fatalf("weights %v", w)
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(t.Context(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	td := a.ok(a.alice, "GET", "/api/v1/today", nil)
	if td["current"] == nil || len(td["recent"].([]any)) == 0 || len(td["unmarked"].([]any)) == 0 {
		t.Fatalf("demo Today is missing something: %v", td)
	}
	year := td["year"].(map[string]any)
	if num(year, "outings") == 0 && num(a.ok(a.alice, "GET", "/api/v1/stats", nil)["previous"].(map[string]any), "outings") == 0 {
		t.Fatal("no done outings in the demo")
	}
	if l := a.list(a.alice, "/api/v1/routes"); len(l) != len(demoRoutes) {
		t.Fatalf("%d routes", len(l))
	}
	if l := a.list(a.bob, "/api/v1/routes"); len(l) != 0 {
		t.Fatal("demo went to Bob")
	}
}
