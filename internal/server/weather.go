package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// dayWeather is the forecast for an outing's day at the start of its route,
// kept on the outing (so it can still be read on the way, and afterwards).
type dayWeather struct {
	Date       string        `json:"date"`
	Code       int           `json:"code"`
	Condition  string        `json:"condition"`
	Text       string        `json:"text,omitempty"`
	TMax       *float64      `json:"t_max"`
	TMin       *float64      `json:"t_min"`
	Precip     *float64      `json:"precip"`
	PrecipProb *float64      `json:"precip_prob"`
	Wind       *float64      `json:"wind"`
	Gust       *float64      `json:"gust"`
	UV         *float64      `json:"uv"`
	Sunrise    string        `json:"sunrise"`
	Sunset     string        `json:"sunset"`
	Hours      []hourWeather `json:"hours,omitempty"`
	Elevation  *float64      `json:"elevation,omitempty"` // of the forecast grid cell
	Source     string        `json:"source"`              // weather (Pocket Weather) | open-meteo
	FetchedAt  string        `json:"fetched_at"`
}

type hourWeather struct {
	Time       string   `json:"time"` // HH:MM
	T          *float64 `json:"t"`
	PrecipProb *float64 `json:"precip_prob"`
	Code       int      `json:"code"`
	Gust       *float64 `json:"gust"`
}

// advice is one line of "what to know" for an outing.
type advice struct {
	Level string `json:"level"` // info | caution | warning
	Icon  string `json:"icon"`
	Text  string `json:"text"`
}

// forecastDays is how far ahead a forecast is worth asking for.
const forecastDays = 9

// refreshWeather gets the forecast for an outing when it is in the next
// days and the one kept is older than an hour (or when forced). It says
// why not, when it could not.
func (s *Server) refreshWeather(ctx context.Context, c *clients, o *store.Outing, r *store.Route, force bool) (string, error) {
	if r == nil || r.StartLat == nil || r.StartLng == nil {
		return "the outing has no route with a start on the map", nil
	}
	d := daysBetween(today(), o.Date)
	if d < 0 || d > forecastDays {
		return "", nil
	}
	if len(o.Weather) > 0 && !force {
		var cur dayWeather
		if json.Unmarshal(o.Weather, &cur) == nil && cur.Date == o.Date {
			if t, err := time.Parse(time.RFC3339, cur.FetchedAt); err == nil && time.Since(t) < time.Hour {
				return "", nil
			}
		}
	}
	w, why, err := s.forecastFor(ctx, c, *r.StartLat, *r.StartLng, o.Date)
	if w == nil {
		return why, err
	}
	w.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(w)
	o.Weather = b
	return "", s.store.SetOutingWeather(ctx, c.u.ID, o.ID, string(b))
}

// forecastFor asks Pocket Weather (or, when it is not connected and the
// person allows it, Open-Meteo directly) for one day at one spot.
func (s *Server) forecastFor(ctx context.Context, c *clients, lat, lng float64, date string) (*dayWeather, string, error) {
	sib, err := optional(c.conn(ctx, appWeather, scopeWeatherRead))
	if err != nil {
		return nil, "", err
	}
	if sib != nil {
		v, err := getJSONFrom(ctx, sib, "/api/v1/forecast", url.Values{"lat": {fmt.Sprint(lat)}, "lon": {fmt.Sprint(lng)}, "hours": {"240"}})
		if err != nil {
			return nil, "", fmt.Errorf("Pocket Weather: %w", err)
		}
		m, _ := v.(map[string]any)
		for _, day := range rows(m["days"]) {
			if str(day, "date") != date {
				continue
			}
			w := &dayWeather{Date: date, Code: int(num(day, "code")), Condition: str(day, "condition"), Text: str(day, "text"),
				TMax: fptr(day, "t_max"), TMin: fptr(day, "t_min"), Precip: fptr(day, "precip"), PrecipProb: fptr(day, "precip_prob"),
				Wind: fptr(day, "wind"), Gust: fptr(day, "gust"), UV: fptr(day, "uv"), Sunrise: hhmm(str(day, "sunrise")), Sunset: hhmm(str(day, "sunset")),
				Elevation: fptr(m, "elevation"), Source: "weather"}
			for _, h := range rows(m["hours"]) {
				t := str(h, "time")
				if !strings.HasPrefix(t, date) {
					continue
				}
				w.Hours = append(w.Hours, hourWeather{Time: hhmm(t), T: fptr(h, "temp"), PrecipProb: fptr(h, "precip_prob"), Code: int(num(h, "code")), Gust: fptr(h, "gust")})
			}
			return w, "", nil
		}
		return nil, "Pocket Weather has no forecast for that day yet", nil
	}
	if !s.online(c.st) {
		return nil, "connect Pocket Weather (or switch on looking things up online) for the forecast", nil
	}
	w, err := s.openMeteoDay(ctx, lat, lng, date)
	if err != nil {
		return nil, "", fmt.Errorf("the forecast: %w", err)
	}
	return w, "", nil
}

func fptr(m map[string]any, k string) *float64 {
	if m == nil {
		return nil
	}
	if v, ok := m[k].(float64); ok {
		return &v
	}
	return nil
}

// sunFor is sunrise and sunset at the start of a route on a day, worked
// out here (no network), as HH:MM.
func sunFor(r *store.Route, date string) (rise, set string) {
	if r == nil || r.StartLat == nil || r.StartLng == nil {
		return "", ""
	}
	d, err := store.ParseDate(date)
	if err != nil {
		return "", ""
	}
	a, b, ok := geo.Sun(d, *r.StartLat, *r.StartLng, time.Local)
	if !ok {
		return "", ""
	}
	return a.Format("15:04"), b.Format("15:04")
}

// finishAt is when an outing would end at the route's time plus a fifth
// for stops.
func finishAt(o *store.Outing) time.Time {
	st := startAt(o)
	if st.IsZero() || o.Minutes <= 0 || o.StartTime == "" {
		return time.Time{}
	}
	return st.Add(time.Duration(float64(o.Minutes)*1.15) * time.Minute)
}

// adviceFor is what to know before an outing: daylight, rain, storms,
// wind, heat and frost (colder at the top), sun, and water on the way.
func adviceFor(o *store.Outing, r *store.Route, st store.Settings) []advice {
	out := []advice{}
	var w dayWeather
	hasW := len(o.Weather) > 0 && json.Unmarshal(o.Weather, &w) == nil && w.Date == o.Date
	rise, set := sunFor(r, o.Date)
	if hasW && w.Sunset != "" {
		rise, set = firstNonEmpty(w.Sunrise, rise), w.Sunset
	}
	// daylight
	if fin := finishAt(o); !fin.IsZero() && set != "" {
		sunset, err := time.ParseInLocation("2006-01-02 15:04", o.Date+" "+set, time.Local)
		if err == nil {
			switch late := fin.Sub(sunset); {
			case late > 0:
				out = append(out, advice{"warning", "sunset", fmt.Sprintf("You would finish around %s, after sunset (%s). Start earlier, take a shorter way, or take a head torch.", fin.Format("15:04"), set)})
			case late > -45*time.Minute:
				out = append(out, advice{"caution", "sunset", fmt.Sprintf("You would finish around %s, close to sunset (%s): take a head torch.", fin.Format("15:04"), set)})
			}
		}
		if st := startAt(o); rise != "" && o.StartTime != "" && st.Format("15:04") < rise {
			out = append(out, advice{"info", "sunrise", "You start before sunrise (" + rise + "): a head torch for the first part."})
		}
	}
	if hasW {
		from, to := o.StartTime, ""
		if fin := finishAt(o); !fin.IsZero() {
			to = fin.Format("15:04")
		}
		var probMax, gustMax float64
		var probAt string
		storm := false
		var tAtStart *float64
		for _, h := range w.Hours {
			if from != "" && (h.Time < from[:2]+":00" || (to != "" && h.Time > to)) {
				continue
			}
			if tAtStart == nil && h.T != nil {
				tAtStart = h.T
			}
			if h.PrecipProb != nil && *h.PrecipProb > probMax {
				probMax, probAt = *h.PrecipProb, h.Time
			}
			if h.Gust != nil && *h.Gust > gustMax {
				gustMax = *h.Gust
			}
			if h.Code >= 95 {
				storm = true
			}
		}
		if len(w.Hours) == 0 {
			if w.PrecipProb != nil {
				probMax = *w.PrecipProb
			}
			if w.Gust != nil {
				gustMax = *w.Gust
			}
			storm = w.Code >= 95
		}
		if storm {
			out = append(out, advice{"warning", "storm", "Thunderstorms are possible: stay off ridges and summits when they build, usually in the afternoon."})
		}
		switch {
		case probMax >= 60:
			at := ""
			if probAt != "" {
				at = " around " + probAt
			}
			out = append(out, advice{"caution", "rain", fmt.Sprintf("Rain is likely (%.0f%%)%s: a rain jacket, and paths may be slippery.", probMax, at)})
		case probMax >= 35:
			out = append(out, advice{"info", "rain", fmt.Sprintf("Some chance of rain (%.0f%%): pack a rain jacket.", probMax)})
		}
		switch {
		case gustMax >= 60:
			out = append(out, advice{"warning", "wind", fmt.Sprintf("Gusts up to %.0f km/h: exposed ridges will be hard going.", gustMax)})
		case gustMax >= 45:
			out = append(out, advice{"caution", "wind", fmt.Sprintf("Windy, gusts up to %.0f km/h: a windproof layer.", gustMax)})
		}
		if w.TMax != nil && *w.TMax >= 30 {
			litres := math.Max(1.5, math.Round(float64(o.Minutes)/60*0.75*2)/2)
			out = append(out, advice{"caution", "heat", fmt.Sprintf("Hot (%.0f°): start early, take shade breaks and about %.1f l of water.", *w.TMax, litres)})
		}
		// colder (about 6.5° a kilometre) at the highest point
		base := w.Elevation
		if base == nil && r != nil && len(r.Track) > 0 {
			base = r.Track[0].Ele
		}
		if r != nil && r.MaxEle != nil && base != nil && *r.MaxEle-*base >= 300 {
			t := tAtStart
			if t == nil {
				t = w.TMax
			}
			drop := (*r.MaxEle - *base) * 6.5 / 1000
			line := fmt.Sprintf("At the highest point (%s) it is about %.0f° colder than below", fmtHeight(*r.MaxEle, st.Units), drop)
			level := "info"
			if t != nil {
				top := *t - drop
				line += fmt.Sprintf(": around %.0f°", top)
				if top <= 0 {
					line += " — ice on the path is possible"
					level = "caution"
				}
			}
			out = append(out, advice{level, "summit", line + "."})
		} else if w.TMin != nil && *w.TMin <= 0 {
			out = append(out, advice{"caution", "cold", fmt.Sprintf("Frost (down to %.0f°): ice on the path is possible early on.", *w.TMin)})
		}
		if w.UV != nil && *w.UV >= 7 {
			out = append(out, advice{"info", "sun", "Strong sun (UV " + fmt.Sprintf("%.0f", *w.UV) + "): sun cream and a hat."})
		}
	}
	// water on the way
	if r != nil && o.Minutes >= 180 && (o.Activity == "hike" || o.Activity == "walk" || o.Activity == "run" || o.Activity == "ski") {
		water := 0
		for _, wp := range r.Waypoints {
			if wp.Kind == "water" || wp.Kind == "hut" || wp.Kind == "food" {
				water++
			}
		}
		if water == 0 {
			litres := math.Max(1, math.Round(float64(o.Minutes)/60*0.5*2)/2)
			out = append(out, advice{"info", "water", fmt.Sprintf("No water marked along the way: carry what you need (about %.1f l for %s).", litres, fmtMinutes(o.Minutes))})
		}
	}
	return out
}
