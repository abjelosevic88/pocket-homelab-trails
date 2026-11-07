package geo

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// Parsed is one route found in a file.
type Parsed struct {
	Name      string
	Desc      string
	Activity  string // a guess from the file ("" = no idea)
	Link      string
	Track     []store.Pt
	Waypoints []ParsedWpt
}

// ParsedWpt is a spot found in a file.
type ParsedWpt struct {
	Name, Note, Kind string
	Lat, Lng         float64
	Ele              *float64
}

// ErrNoTrack means the file had nothing to walk.
var ErrNoTrack = errors.New("there is no track or route in this file")

// ParseFile reads a GPX, KML, TCX or GeoJSON file (by its content, the
// name only breaks ties). Every track is a route; spots go to the route
// they lie along (or to the only route).
func ParseFile(name string, data []byte) ([]Parsed, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	trim := bytes.TrimSpace(data)
	lower := strings.ToLower(name)
	var out []Parsed
	var wpts []ParsedWpt
	var err error
	switch {
	case len(trim) > 0 && (trim[0] == '{' || strings.HasSuffix(lower, ".geojson") || strings.HasSuffix(lower, ".json")):
		out, wpts, err = parseGeoJSON(trim)
	case bytes.Contains(firstK(trim), []byte("<gpx")):
		out, wpts, err = parseGPX(trim)
	case bytes.Contains(firstK(trim), []byte("<kml")):
		out, wpts, err = parseKML(trim)
	case bytes.Contains(firstK(trim), []byte("TrainingCenterDatabase")):
		out, err = parseTCX(trim)
	case strings.HasSuffix(lower, ".gpx"):
		out, wpts, err = parseGPX(trim)
	case strings.HasSuffix(lower, ".kml"):
		out, wpts, err = parseKML(trim)
	default:
		return nil, errors.New("this is not a GPX, KML, TCX or GeoJSON file")
	}
	if err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, p := range out {
		if len(p.Track) >= 2 {
			kept = append(kept, p)
		}
	}
	out = kept
	if len(out) == 0 {
		return nil, ErrNoTrack
	}
	base := strings.TrimSuffix(name, fileExt(name))
	for i := range out {
		if strings.TrimSpace(out[i].Name) == "" {
			out[i].Name = base
			if len(out) > 1 {
				out[i].Name = fmt.Sprintf("%s (%d)", base, i+1)
			}
		}
		out[i].Name = clean(out[i].Name, 200)
		out[i].Desc = clean(out[i].Desc, 4000)
	}
	for _, w := range wpts {
		best, bestOff := 0, math.Inf(1)
		for i, p := range out {
			if _, off := Along(p.Track, w.Lat, w.Lng); off < bestOff {
				best, bestOff = i, off
			}
		}
		if len(out) == 1 || bestOff < 2000 {
			out[best].Waypoints = append(out[best].Waypoints, w)
		}
	}
	return out, nil
}

func firstK(b []byte) []byte {
	if len(b) > 1024 {
		return b[:1024]
	}
	return b
}

func fileExt(n string) string {
	if i := strings.LastIndexByte(n, '.'); i > 0 && len(n)-i <= 8 {
		return n[i:]
	}
	return ""
}

func clean(s string, n int) string {
	s = strings.TrimSpace(html.UnescapeString(stripTags(s)))
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func stripTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- GPX ----

type gpxDoc struct {
	Meta struct {
		Name string `xml:"name"`
		Desc string `xml:"desc"`
	} `xml:"metadata"`
	Wpt []gpxPt `xml:"wpt"`
	Rte []struct {
		Name string  `xml:"name"`
		Desc string  `xml:"desc"`
		Type string  `xml:"type"`
		Link gpxLink `xml:"link"`
		Pts  []gpxPt `xml:"rtept"`
	} `xml:"rte"`
	Trk []struct {
		Name string  `xml:"name"`
		Desc string  `xml:"desc"`
		Type string  `xml:"type"`
		Link gpxLink `xml:"link"`
		Seg  []struct {
			Pts []gpxPt `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
}

type gpxLink struct {
	Href string `xml:"href,attr"`
}

type gpxPt struct {
	Lat  string `xml:"lat,attr"`
	Lon  string `xml:"lon,attr"`
	Ele  string `xml:"ele"`
	Time string `xml:"time"`
	Name string `xml:"name"`
	Desc string `xml:"desc"`
	Cmt  string `xml:"cmt"`
	Sym  string `xml:"sym"`
	Type string `xml:"type"`
}

func (g gpxPt) pt() (store.Pt, bool) {
	lat, e1 := strconv.ParseFloat(strings.TrimSpace(g.Lat), 64)
	lng, e2 := strconv.ParseFloat(strings.TrimSpace(g.Lon), 64)
	if e1 != nil || e2 != nil || !validLL(lat, lng) {
		return store.Pt{}, false
	}
	p := store.Pt{Lat: lat, Lng: lng}
	if v, err := strconv.ParseFloat(strings.TrimSpace(g.Ele), 64); err == nil && v > -500 && v < 9000 {
		p.Ele = &v
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(g.Time)); err == nil {
		p.T = t.Unix()
	}
	return p, true
}

func validLL(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !(lat == 0 && lng == 0)
}

func xmlDecoder(b []byte) *xml.Decoder {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return dec
}

func parseGPX(b []byte) ([]Parsed, []ParsedWpt, error) {
	var doc gpxDoc
	if err := xmlDecoder(b).Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("the GPX file could not be read: %w", err)
	}
	var out []Parsed
	for _, t := range doc.Trk {
		p := Parsed{Name: firstOf(t.Name, doc.Meta.Name), Desc: firstOf(t.Desc, doc.Meta.Desc), Activity: guessActivity(t.Type + " " + t.Name), Link: t.Link.Href}
		for _, s := range t.Seg {
			for _, g := range s.Pts {
				if pt, ok := g.pt(); ok {
					p.Track = append(p.Track, pt)
				}
			}
		}
		out = append(out, p)
	}
	for _, r := range doc.Rte {
		p := Parsed{Name: firstOf(r.Name, doc.Meta.Name), Desc: firstOf(r.Desc, doc.Meta.Desc), Activity: guessActivity(r.Type + " " + r.Name), Link: r.Link.Href}
		for _, g := range r.Pts {
			if pt, ok := g.pt(); ok {
				p.Track = append(p.Track, pt)
			}
		}
		// a route next to a track of the same name is usually the same line
		if len(p.Track) >= 2 && !sameAsOne(out, p) {
			out = append(out, p)
		}
	}
	var wpts []ParsedWpt
	for _, g := range doc.Wpt {
		pt, ok := g.pt()
		if !ok {
			continue
		}
		wpts = append(wpts, ParsedWpt{Name: clean(g.Name, 200), Note: clean(firstOf(g.Desc, g.Cmt), 2000), Kind: GuessKind(g.Sym + " " + g.Type + " " + g.Name), Lat: pt.Lat, Lng: pt.Lng, Ele: pt.Ele})
	}
	return out, wpts, nil
}

func sameAsOne(have []Parsed, p Parsed) bool {
	for _, h := range have {
		if len(h.Track) > 0 && d(h.Track[0], p.Track[0]) < 50 && d(h.Track[len(h.Track)-1], p.Track[len(p.Track)-1]) < 50 {
			return true
		}
	}
	return false
}

func firstOf(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// ---- KML (Google Earth, My Maps) ----

func parseKML(b []byte) ([]Parsed, []ParsedWpt, error) {
	dec := xmlDecoder(b)
	var out []Parsed
	var wpts []ParsedWpt
	var stack []string
	var name, desc string
	var lines [][]store.Pt
	var point *store.Pt
	var whens []string
	var coords []string
	var text strings.Builder
	inPlacemark := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("the KML file could not be read: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
			if t.Name.Local == "Placemark" {
				inPlacemark, name, desc, lines, point, whens, coords = true, "", "", nil, nil, nil, nil
			}
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			el := t.Name.Local
			parent := ""
			if len(stack) >= 2 {
				parent = stack[len(stack)-2]
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			v := strings.TrimSpace(text.String())
			if !inPlacemark {
				continue
			}
			switch el {
			case "name":
				if parent == "Placemark" {
					name = v
				}
			case "description":
				if parent == "Placemark" {
					desc = v
				}
			case "coordinates":
				pts := kmlCoords(v)
				if parent == "Point" && len(pts) > 0 {
					point = &pts[0]
				} else if len(pts) > 0 {
					lines = append(lines, pts)
				}
			case "when":
				whens = append(whens, v)
			case "coord":
				coords = append(coords, v)
			case "Track":
				var tr []store.Pt
				for i, c := range coords {
					f := strings.Fields(c)
					if len(f) < 2 {
						continue
					}
					lng, e1 := strconv.ParseFloat(f[0], 64)
					lat, e2 := strconv.ParseFloat(f[1], 64)
					if e1 != nil || e2 != nil || !validLL(lat, lng) {
						continue
					}
					p := store.Pt{Lat: lat, Lng: lng}
					if len(f) > 2 {
						if e, err := strconv.ParseFloat(f[2], 64); err == nil {
							p.Ele = &e
						}
					}
					if i < len(whens) {
						if tt, err := time.Parse(time.RFC3339, whens[i]); err == nil {
							p.T = tt.Unix()
						}
					}
					tr = append(tr, p)
				}
				if len(tr) > 0 {
					lines = append(lines, tr)
				}
				coords, whens = nil, nil
			case "Placemark":
				inPlacemark = false
				if len(lines) > 0 {
					p := Parsed{Name: name, Desc: desc, Activity: guessActivity(name)}
					for _, l := range lines {
						p.Track = append(p.Track, l...)
					}
					out = append(out, p)
				} else if point != nil {
					wpts = append(wpts, ParsedWpt{Name: clean(name, 200), Note: clean(desc, 2000), Kind: GuessKind(name), Lat: point.Lat, Lng: point.Lng, Ele: point.Ele})
				}
			}
		}
	}
	return out, wpts, nil
}

func kmlCoords(s string) []store.Pt {
	var out []store.Pt
	for _, f := range strings.Fields(s) {
		parts := strings.Split(f, ",")
		if len(parts) < 2 {
			continue
		}
		lng, e1 := strconv.ParseFloat(parts[0], 64)
		lat, e2 := strconv.ParseFloat(parts[1], 64)
		if e1 != nil || e2 != nil || !validLL(lat, lng) {
			continue
		}
		p := store.Pt{Lat: lat, Lng: lng}
		if len(parts) > 2 {
			if e, err := strconv.ParseFloat(parts[2], 64); err == nil && e != 0 {
				p.Ele = &e
			}
		}
		out = append(out, p)
	}
	return out
}

// ---- TCX (Garmin) ----

func parseTCX(b []byte) ([]Parsed, error) {
	var doc struct {
		Activities []struct {
			Sport string `xml:"Sport,attr"`
			Laps  []struct {
				Track []struct {
					Pts []struct {
						Time string   `xml:"Time"`
						Lat  float64  `xml:"Position>LatitudeDegrees"`
						Lng  float64  `xml:"Position>LongitudeDegrees"`
						Alt  *float64 `xml:"AltitudeMeters"`
					} `xml:"Trackpoint"`
				} `xml:"Track"`
			} `xml:"Lap"`
		} `xml:"Activities>Activity"`
		Courses []struct {
			Name  string `xml:"Name"`
			Track []struct {
				Pts []struct {
					Lat float64  `xml:"Position>LatitudeDegrees"`
					Lng float64  `xml:"Position>LongitudeDegrees"`
					Alt *float64 `xml:"AltitudeMeters"`
				} `xml:"Trackpoint"`
			} `xml:"Track"`
		} `xml:"Courses>Course"`
	}
	if err := xmlDecoder(b).Decode(&doc); err != nil {
		return nil, fmt.Errorf("the TCX file could not be read: %w", err)
	}
	var out []Parsed
	for _, a := range doc.Activities {
		p := Parsed{Activity: guessActivity(a.Sport)}
		for _, l := range a.Laps {
			for _, tr := range l.Track {
				for _, x := range tr.Pts {
					if !validLL(x.Lat, x.Lng) {
						continue
					}
					pt := store.Pt{Lat: x.Lat, Lng: x.Lng, Ele: x.Alt}
					if t, err := time.Parse(time.RFC3339, x.Time); err == nil {
						pt.T = t.Unix()
					}
					p.Track = append(p.Track, pt)
				}
			}
		}
		out = append(out, p)
	}
	for _, c := range doc.Courses {
		p := Parsed{Name: c.Name}
		for _, tr := range c.Track {
			for _, x := range tr.Pts {
				if validLL(x.Lat, x.Lng) {
					p.Track = append(p.Track, store.Pt{Lat: x.Lat, Lng: x.Lng, Ele: x.Alt})
				}
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// ---- GeoJSON ----

type gjGeom struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
	Geometries  []gjGeom        `json:"geometries"`
}

type gjFeature struct {
	Type       string         `json:"type"`
	Geometry   *gjGeom        `json:"geometry"`
	Properties map[string]any `json:"properties"`
	Features   []gjFeature    `json:"features"`
}

func parseGeoJSON(b []byte) ([]Parsed, []ParsedWpt, error) {
	var root gjFeature
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, nil, fmt.Errorf("the GeoJSON file could not be read: %w", err)
	}
	feats := root.Features
	switch root.Type {
	case "Feature":
		feats = []gjFeature{root}
	case "LineString", "MultiLineString", "Point", "GeometryCollection":
		var g gjGeom
		_ = json.Unmarshal(b, &g)
		feats = []gjFeature{{Type: "Feature", Geometry: &g}}
	}
	var out []Parsed
	var wpts []ParsedWpt
	for _, f := range feats {
		if f.Geometry == nil {
			continue
		}
		name, _ := f.Properties["name"].(string)
		desc, _ := f.Properties["description"].(string)
		var lines [][]store.Pt
		var points []store.Pt
		collect(*f.Geometry, &lines, &points)
		if len(lines) > 0 {
			p := Parsed{Name: name, Desc: desc, Activity: guessActivity(name)}
			for _, l := range lines {
				p.Track = append(p.Track, l...)
			}
			out = append(out, p)
		}
		for _, pt := range points {
			wpts = append(wpts, ParsedWpt{Name: clean(name, 200), Note: clean(desc, 2000), Kind: GuessKind(name), Lat: pt.Lat, Lng: pt.Lng, Ele: pt.Ele})
		}
	}
	return out, wpts, nil
}

func collect(g gjGeom, lines *[][]store.Pt, points *[]store.Pt) {
	pos := func(a []float64) (store.Pt, bool) {
		if len(a) < 2 || !validLL(a[1], a[0]) {
			return store.Pt{}, false
		}
		p := store.Pt{Lat: a[1], Lng: a[0]}
		if len(a) > 2 {
			e := a[2]
			p.Ele = &e
		}
		return p, true
	}
	switch g.Type {
	case "Point":
		var a []float64
		if json.Unmarshal(g.Coordinates, &a) == nil {
			if p, ok := pos(a); ok {
				*points = append(*points, p)
			}
		}
	case "LineString":
		var a [][]float64
		if json.Unmarshal(g.Coordinates, &a) == nil {
			var l []store.Pt
			for _, c := range a {
				if p, ok := pos(c); ok {
					l = append(l, p)
				}
			}
			*lines = append(*lines, l)
		}
	case "MultiLineString":
		var a [][][]float64
		if json.Unmarshal(g.Coordinates, &a) == nil {
			for _, line := range a {
				var l []store.Pt
				for _, c := range line {
					if p, ok := pos(c); ok {
						l = append(l, p)
					}
				}
				*lines = append(*lines, l)
			}
		}
	case "GeometryCollection":
		for _, sub := range g.Geometries {
			collect(sub, lines, points)
		}
	}
}

// ---- guesses ----

func guessActivity(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "mountain") && strings.Contains(s, "bik"), strings.Contains(s, "mtb"):
		return "mtb"
	case strings.Contains(s, "gravel"):
		return "gravel"
	case strings.Contains(s, "cycl"), strings.Contains(s, "bik"), strings.Contains(s, "ride"), strings.Contains(s, "road"):
		return "bike"
	case strings.Contains(s, "run"), strings.Contains(s, "jog"):
		return "run"
	case strings.Contains(s, "ski"), strings.Contains(s, "snowshoe"):
		return "ski"
	case strings.Contains(s, "walk"):
		return "walk"
	case strings.Contains(s, "hik"), strings.Contains(s, "trek"), strings.Contains(s, "mountaineer"):
		return "hike"
	}
	return ""
}

// GuessKind maps a GPX symbol or a name to a waypoint kind.
func GuessKind(s string) string {
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
	case has("water", "spring", "fountain", "izvor", "česma", "cesma", "quelle", "brunnen"):
		return "water"
	case has("hut", "lodge", "lodging", "refuge", "rifugio", "hütte", "dom ", "koliba", "planinarski"):
		return "hut"
	case has("summit", "peak", "vrh", "gipfel", "top"):
		return "summit"
	case has("parking", "car park", "trailhead"):
		return "parking"
	case has("view", "scenic", "vidikovac", "aussicht"):
		return "viewpoint"
	case has("restaurant", "food", "café", "cafe", "bar", "inn"):
		return "food"
	case has("camp", "tent", "bivouac", "bivak"):
		return "camp"
	case has("shelter", "skloni"):
		return "shelter"
	case has("lake", "jezero", "see "):
		return "lake"
	case has("bus", "train", "station", "stop", "ferry", "cable car", "gondola"):
		return "transport"
	case has("danger", "warning", "caution", "exposed", "scramble"):
		return "danger"
	case has("junction", "crossing", "fork"):
		return "junction"
	case has("photo"):
		return "photo"
	}
	return "other"
}

// ---- writing GPX ----

// WriteGPX writes a route (and its spots) as GPX 1.1, readable by every
// map app (OsmAnd, Organic Maps, Komoot, Garmin).
func WriteGPX(w io.Writer, name, desc, activity string, track []store.Pt, wpts []*store.Waypoint) error {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx version="1.1" creator="Pocket Trails" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
	b.WriteString("  <metadata><name>" + esc(name) + "</name>")
	if desc != "" {
		b.WriteString("<desc>" + esc(desc) + "</desc>")
	}
	b.WriteString("</metadata>\n")
	for _, wp := range wpts {
		fmt.Fprintf(&b, `  <wpt lat="%.6f" lon="%.6f">`, wp.Lat, wp.Lng)
		if wp.Ele != nil {
			fmt.Fprintf(&b, "<ele>%.1f</ele>", *wp.Ele)
		}
		b.WriteString("<name>" + esc(firstOf(wp.Name, wp.Kind)) + "</name>")
		if wp.Note != "" {
			b.WriteString("<desc>" + esc(wp.Note) + "</desc>")
		}
		b.WriteString("<sym>" + esc(gpxSym(wp.Kind)) + "</sym><type>" + esc(wp.Kind) + "</type></wpt>\n")
	}
	b.WriteString("  <trk><name>" + esc(name) + "</name><type>" + esc(activity) + "</type><trkseg>\n")
	for _, p := range track {
		fmt.Fprintf(&b, `    <trkpt lat="%.6f" lon="%.6f">`, p.Lat, p.Lng)
		if p.Ele != nil {
			fmt.Fprintf(&b, "<ele>%.1f</ele>", *p.Ele)
		}
		if p.T > 0 {
			b.WriteString("<time>" + time.Unix(p.T, 0).UTC().Format(time.RFC3339) + "</time>")
		}
		b.WriteString("</trkpt>\n")
	}
	b.WriteString("  </trkseg></trk>\n</gpx>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func gpxSym(kind string) string {
	switch kind {
	case "water":
		return "Drinking Water"
	case "hut":
		return "Lodging"
	case "food":
		return "Restaurant"
	case "shelter":
		return "Shelter"
	case "camp":
		return "Campground"
	case "summit":
		return "Summit"
	case "viewpoint":
		return "Scenic Area"
	case "lake":
		return "Swimming Area"
	case "parking":
		return "Parking Area"
	case "transport":
		return "Ground Transportation"
	case "danger":
		return "Danger Area"
	case "junction":
		return "Crossing"
	case "photo":
		return "Scenic Area"
	}
	return "Flag, Blue"
}
