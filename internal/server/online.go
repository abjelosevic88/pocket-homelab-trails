package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// ---- the free online services ----
//
// Place search (OpenStreetMap's Nominatim, one request a second, cached),
// heights (Open-Meteo's elevation model), paths that follow the trails
// (BRouter) and, when Pocket Weather is not connected, the forecast
// (Open-Meteo). None needs a key; each person can switch them off
// (Settings → General), and DEMO_DATA never calls out.

var errOffline = &userErr{"looking things up online is switched off (Settings → General)"}

func (s *Server) online(st store.Settings) bool { return st.Online && !s.cfg.DemoData }

func (s *Server) getOnline(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := s.httpc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("HTTP %d %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(v)
}

// ---- place search ----

type foundPlace struct {
	Name   string    `json:"name"`
	Detail string    `json:"detail"`
	Lat    float64   `json:"lat"`
	Lng    float64   `json:"lng"`
	Kind   string    `json:"kind"`
	Ele    *int      `json:"ele,omitempty"`
	BBox   []float64 `json:"bbox,omitempty"`
}

func (s *Server) searchPlaces(ctx context.Context, q string) ([]foundPlace, error) {
	key := "nominatim:" + strings.ToLower(q)
	if v, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 30*24*time.Hour {
			var out []foundPlace
			if json.Unmarshal([]byte(v), &out) == nil {
				return out, nil
			}
		}
	}
	s.lookMu.Lock()
	if wait := time.Second - time.Since(s.lookAt); wait > 0 {
		time.Sleep(wait)
	}
	s.lookAt = time.Now()
	s.lookMu.Unlock()
	var raw []struct {
		Name        string            `json:"name"`
		DisplayName string            `json:"display_name"`
		Lat         string            `json:"lat"`
		Lon         string            `json:"lon"`
		Category    string            `json:"category"`
		Type        string            `json:"type"`
		BBox        []string          `json:"boundingbox"`
		Extra       map[string]string `json:"extratags"`
	}
	u := "https://nominatim.openstreetmap.org/search?format=jsonv2&extratags=1&limit=8&q=" + url.QueryEscape(q)
	if err := s.getOnline(ctx, u, &raw); err != nil {
		return nil, err
	}
	out := []foundPlace{}
	for _, x := range raw {
		lat, _ := strconv.ParseFloat(x.Lat, 64)
		lng, _ := strconv.ParseFloat(x.Lon, 64)
		p := foundPlace{Name: firstNonEmpty(x.Name, strings.SplitN(x.DisplayName, ",", 2)[0]), Detail: x.DisplayName, Lat: lat, Lng: lng, Kind: x.Type}
		if e, err := strconv.Atoi(strings.TrimSuffix(x.Extra["ele"], " m")); err == nil {
			p.Ele = &e
		}
		if len(x.BBox) == 4 {
			for _, b := range x.BBox {
				f, _ := strconv.ParseFloat(b, 64)
				p.BBox = append(p.BBox, f)
			}
		}
		out = append(out, p)
	}
	_ = s.store.CachePut(ctx, key, toJSONs(out))
	return out, nil
}

// ---- heights ----

// fillHeights gives every point of a track a height from Open-Meteo's
// elevation model (90 m grid; good for the climb of a walk, not for a
// cliff edge). Points that have one keep it.
func (s *Server) fillHeights(ctx context.Context, track []store.Pt) error {
	var need []int
	for i, p := range track {
		if p.Ele == nil {
			need = append(need, i)
		}
	}
	if len(need) == 0 {
		return nil
	}
	if len(need) > 3000 {
		return &userErr{"the route has too many points to look up heights for"}
	}
	for start := 0; start < len(need); start += 100 {
		batch := need[start:min(start+100, len(need))]
		var lats, lngs []string
		for _, i := range batch {
			lats = append(lats, strconv.FormatFloat(track[i].Lat, 'f', 5, 64))
			lngs = append(lngs, strconv.FormatFloat(track[i].Lng, 'f', 5, 64))
		}
		var out struct {
			Elevation []float64 `json:"elevation"`
		}
		u := "https://api.open-meteo.com/v1/elevation?latitude=" + strings.Join(lats, ",") + "&longitude=" + strings.Join(lngs, ",")
		if err := s.getOnline(ctx, u, &out); err != nil {
			return fmt.Errorf("heights: %w", err)
		}
		for j, i := range batch {
			if j < len(out.Elevation) && !math.IsNaN(out.Elevation[j]) {
				v := out.Elevation[j]
				track[i].Ele = &v
			}
		}
	}
	return nil
}

// densify adds points every step metres along straight lines (a drawn
// route), so the heights between the clicks are counted.
func densify(pts []store.Pt, step float64) []store.Pt {
	if len(pts) < 2 {
		return pts
	}
	out := []store.Pt{pts[0]}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := geo.Dist(a.Lat, a.Lng, b.Lat, b.Lng)
		n := int(d / step)
		for k := 1; k < n; k++ {
			f := float64(k) / float64(n)
			out = append(out, store.Pt{Lat: a.Lat + (b.Lat-a.Lat)*f, Lng: a.Lng + (b.Lng-a.Lng)*f})
		}
		out = append(out, b)
	}
	return out
}

// ---- paths that follow the trails ----

// brouterProfile is the BRouter profile for an activity.
func brouterProfile(activity string) string {
	switch activity {
	case "bike":
		return "fastbike"
	case "gravel", "mtb":
		return "trekking"
	}
	return "hiking-mountain"
}

// followPaths asks BRouter for the way along paths and roads through the
// drawn points; the answer has heights.
func (s *Server) followPaths(ctx context.Context, controls [][]float64, activity string) ([]store.Pt, error) {
	if len(controls) < 2 {
		return nil, &userErr{"put at least two points on the map"}
	}
	if len(controls) > 60 {
		return nil, &userErr{"that is too many points for one route (60 at most)"}
	}
	var ll []string
	for _, c := range controls {
		if len(c) < 2 {
			return nil, &userErr{"a point has no position"}
		}
		ll = append(ll, strconv.FormatFloat(c[1], 'f', 6, 64)+","+strconv.FormatFloat(c[0], 'f', 6, 64))
	}
	u := "https://brouter.de/brouter?alternativeidx=0&format=geojson&profile=" + brouterProfile(activity) + "&lonlats=" + url.QueryEscape(strings.Join(ll, "|"))
	var out struct {
		Features []struct {
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.getOnline(ctx, u, &out); err != nil {
		if strings.Contains(err.Error(), "HTTP 4") || strings.Contains(err.Error(), "HTTP 5") {
			return nil, &userErr{"no way along paths was found between those points; move a point or draw straight lines"}
		}
		return nil, err
	}
	if len(out.Features) == 0 {
		return nil, errors.New("no route came back")
	}
	var track []store.Pt
	for _, c := range out.Features[0].Geometry.Coordinates {
		if len(c) < 2 {
			continue
		}
		p := store.Pt{Lat: c[1], Lng: c[0]}
		if len(c) > 2 {
			e := c[2]
			p.Ele = &e
		}
		track = append(track, p)
	}
	return track, nil
}

// ---- the forecast, when Pocket Weather is not connected ----

func (s *Server) openMeteoDay(ctx context.Context, lat, lng float64, date string) (*dayWeather, error) {
	key := fmt.Sprintf("om:%.2f,%.2f:%s", lat, lng, date)
	if v, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < time.Hour {
			var w dayWeather
			if json.Unmarshal([]byte(v), &w) == nil {
				return &w, nil
			}
		}
	}
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&timezone=auto&start_date=%s&end_date=%s"+
		"&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,precipitation_probability_max,wind_speed_10m_max,wind_gusts_10m_max,uv_index_max,sunrise,sunset"+
		"&hourly=temperature_2m,precipitation_probability,weather_code,wind_gusts_10m", lat, lng, date, date)
	var out struct {
		Elevation float64 `json:"elevation"`
		Daily     struct {
			Code []int      `json:"weather_code"`
			TMax []*float64 `json:"temperature_2m_max"`
			TMin []*float64 `json:"temperature_2m_min"`
			Rain []*float64 `json:"precipitation_sum"`
			Prob []*float64 `json:"precipitation_probability_max"`
			Wind []*float64 `json:"wind_speed_10m_max"`
			Gust []*float64 `json:"wind_gusts_10m_max"`
			UV   []*float64 `json:"uv_index_max"`
			Rise []string   `json:"sunrise"`
			Set  []string   `json:"sunset"`
		} `json:"daily"`
		Hourly struct {
			Time []string   `json:"time"`
			T    []*float64 `json:"temperature_2m"`
			Prob []*float64 `json:"precipitation_probability"`
			Code []int      `json:"weather_code"`
			Gust []*float64 `json:"wind_gusts_10m"`
		} `json:"hourly"`
	}
	if err := s.getOnline(ctx, u, &out); err != nil {
		return nil, err
	}
	if len(out.Daily.Code) == 0 {
		return nil, errors.New("no forecast for that day yet")
	}
	at := func(a []*float64) *float64 {
		if len(a) > 0 {
			return a[0]
		}
		return nil
	}
	w := &dayWeather{Date: date, Code: out.Daily.Code[0], TMax: at(out.Daily.TMax), TMin: at(out.Daily.TMin), Precip: at(out.Daily.Rain), PrecipProb: at(out.Daily.Prob),
		Wind: at(out.Daily.Wind), Gust: at(out.Daily.Gust), UV: at(out.Daily.UV), Source: "open-meteo", Elevation: &out.Elevation}
	w.Condition = wmoWords(w.Code)
	if len(out.Daily.Rise) > 0 {
		w.Sunrise, w.Sunset = hhmm(out.Daily.Rise[0]), hhmm(out.Daily.Set[0])
	}
	for i, t := range out.Hourly.Time {
		h := hourWeather{Time: hhmm(t)}
		if i < len(out.Hourly.T) {
			h.T = out.Hourly.T[i]
		}
		if i < len(out.Hourly.Prob) {
			h.PrecipProb = out.Hourly.Prob[i]
		}
		if i < len(out.Hourly.Code) {
			h.Code = out.Hourly.Code[i]
		}
		if i < len(out.Hourly.Gust) {
			h.Gust = out.Hourly.Gust[i]
		}
		w.Hours = append(w.Hours, h)
	}
	_ = s.store.CachePut(ctx, key, toJSONs(w))
	return w, nil
}

// hhmm is the time part of "2026-11-07T07:05" (or of an RFC 3339 time).
func hhmm(v string) string {
	if i := strings.IndexByte(v, 'T'); i >= 0 && len(v) >= i+6 {
		return v[i+1 : i+6]
	}
	return v
}

// wmoWords is a WMO weather code in words.
func wmoWords(c int) string {
	switch {
	case c == 0:
		return "Clear"
	case c <= 2:
		return "Partly cloudy"
	case c == 3:
		return "Overcast"
	case c == 45 || c == 48:
		return "Fog"
	case c >= 51 && c <= 57:
		return "Drizzle"
	case c >= 61 && c <= 67:
		return "Rain"
	case c >= 71 && c <= 77:
		return "Snow"
	case c >= 80 && c <= 82:
		return "Showers"
	case c == 85 || c == 86:
		return "Snow showers"
	case c >= 95:
		return "Thunderstorms"
	}
	return "Mixed"
}

func toJSONs(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
