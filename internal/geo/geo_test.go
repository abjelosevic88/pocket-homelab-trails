package geo

import (
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

func e(v float64) *float64 { return &v }

func TestMeasureClimbIgnoresNoise(t *testing.T) {
	var tr []store.Pt
	for i := 0; i < 200; i++ {
		ele := 500.0
		if i%2 == 0 {
			ele = 502 // GPS wobble, not a climb
		}
		tr = append(tr, store.Pt{Lat: 43.8 + float64(i)*0.0003, Lng: 18.4, Ele: e(ele)})
	}
	if s := Measure(tr); s.GainM != 0 {
		t.Fatalf("wobble counted as %v m", s.GainM)
	}
	tr = append(tr, store.Pt{Lat: 43.9, Lng: 18.4, Ele: e(800)})
	if s := Measure(tr); s.GainM < 297 || s.GainM > 301 {
		t.Fatalf("gain %v", s.GainM)
	}
}

func TestMinutesDIN(t *testing.T) {
	// 12 km with 600 m up and down: flat 3 h, height 2 h + 1.2 h = 3.2 h → 3.2 + 1.5 = 4.7 h
	if m := Minutes("hike", 12000, 600, 600, 100); m < 275 || m > 290 {
		t.Fatalf("hike minutes %d", m)
	}
	if m := Minutes("hike", 12000, 600, 600, 200); m > 150 {
		t.Fatalf("twice as fast: %d", m)
	}
	if Minutes("bike", 44000, 0, 0, 100) != 120 {
		t.Fatal("bike")
	}
	if Level("hike", 6000, 200) != "easy" || Level("hike", 22000, 1300) != "hard" || Level("bike", 25000, 300) != "easy" {
		t.Fatal("levels")
	}
}

func TestSunSarajevo(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	rise, set, ok := Sun(time.Date(2026, 6, 21, 0, 0, 0, 0, loc), 43.8563, 18.4131, loc)
	if !ok {
		t.Fatal("no sun")
	}
	if r := rise.Format("15:04"); r < "04:45" || r > "05:05" {
		t.Fatalf("sunrise %s", r)
	}
	if s := set.Format("15:04"); s < "20:15" || s > "20:35" {
		t.Fatalf("sunset %s", s)
	}
}

func TestParseKMLAndGeoJSON(t *testing.T) {
	kml := `<?xml version="1.0"?><kml xmlns="http://www.opengis.net/kml/2.2"><Document><Placemark><name>Lukomir</name><LineString><coordinates>
18.2316,43.6623,1340 18.2232,43.6581,1420 18.1983,43.6448,1470</coordinates></LineString></Placemark>
<Placemark><name>Spring</name><Point><coordinates>18.22,43.658,0</coordinates></Point></Placemark></Document></kml>`
	ps, err := ParseFile("x.kml", []byte(kml))
	if err != nil || len(ps) != 1 || len(ps[0].Track) != 3 || ps[0].Name != "Lukomir" || len(ps[0].Waypoints) != 1 || ps[0].Waypoints[0].Kind != "water" {
		t.Fatalf("kml: %v %+v", err, ps)
	}
	gj := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"name":"Ride"},"geometry":{"type":"LineString","coordinates":[[18.40,43.85],[18.38,43.85],[18.35,43.84]]}}]}`
	ps, err = ParseFile("ride.geojson", []byte(gj))
	if err != nil || len(ps) != 1 || ps[0].Activity != "bike" || len(ps[0].Track) != 3 {
		t.Fatalf("geojson: %v %+v", err, ps)
	}
	if _, err := ParseFile("x.txt", []byte("hello")); err == nil {
		t.Fatal("text taken as a route")
	}
}

func TestCompareAndTrim(t *testing.T) {
	var route []store.Pt
	for i := 0; i <= 100; i++ {
		route = append(route, store.Pt{Lat: 43.8 + float64(i)*0.0005, Lng: 18.4, Ele: e(500 + float64(i)*3)})
	}
	var day []store.Pt
	ts := int64(1_780_000_000)
	// at home first (far away), then the walk, then home again
	day = append(day, store.Pt{Lat: 43.70, Lng: 18.30, T: ts})
	for i, p := range route {
		day = append(day, store.Pt{Lat: p.Lat + 0.00002, Lng: p.Lng, Ele: p.Ele, T: ts + 3600 + int64(i*40)})
	}
	day = append(day, store.Pt{Lat: 43.70, Lng: 18.30, T: ts + 20000})
	walk := Trim(route, day, 300)
	if len(walk) != len(route) {
		t.Fatalf("trimmed to %d, want %d", len(walk), len(route))
	}
	c := Compare(route, walk, "hike", 100)
	if c.CoveredPct < 98 || c.MaxOffM > 10 || c.MovingS < 3900 || c.SpeedPct <= 0 {
		t.Fatalf("comparison %+v", c)
	}
}

func TestWriteGPXRoundTrip(t *testing.T) {
	tr := []store.Pt{{Lat: 43.8, Lng: 18.4, Ele: e(500)}, {Lat: 43.81, Lng: 18.41, Ele: e(600)}}
	var b strings.Builder
	if err := WriteGPX(&b, "A & B", "", "hike", tr, []*store.Waypoint{{Kind: "summit", Name: "Top", Lat: 43.81, Lng: 18.41}}); err != nil {
		t.Fatal(err)
	}
	ps, err := ParseFile("a.gpx", []byte(b.String()))
	if err != nil || len(ps) != 1 || ps[0].Name != "A & B" || len(ps[0].Waypoints) != 1 || ps[0].Waypoints[0].Kind != "summit" {
		t.Fatalf("round trip: %v %+v", err, ps)
	}
}
