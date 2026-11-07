package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

var (
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reTime  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	reCur   = regexp.MustCompile(`^[A-Z]{3}$`)
)

// userErr is a reason the person can act on (400, not 502).
type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

// today is the local day (the box's zone).
func today() string { return time.Now().Format("2006-01-02") }

func (s *Server) settingsOf(ctx context.Context, uid int64) store.Settings {
	st, _ := s.store.GetSettings(ctx, uid)
	return st
}

func (s *Server) userFail(w http.ResponseWriter, err error, what string) {
	var ue *userErr
	if errors.As(err, &ue) {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	if isNotConnected(err) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.fail(w, err, what)
}

// link is a page of Trails as a full address (for messages and events).
func (s *Server) link(path string) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + path
}

// ---- routes ----

// maxTrack is how many points a stored route keeps (plenty for a 100 km
// ride at a few metres; a phone's 1 Hz log is thinned to this).
const maxTrack = 3000

// prepRoute works out everything that follows from a route's track: the
// numbers, the start and end, a thin preview line, and where each spot
// lies along it.
func prepRoute(r *store.Route) {
	r.Track = geo.Thin(r.Track, maxTrack)
	st := geo.Measure(r.Track)
	r.DistanceM, r.GainM, r.LossM = math.Round(st.DistanceM), math.Round(st.GainM), math.Round(st.LossM)
	r.MinEle, r.MaxEle, r.Loop = st.MinEle, st.MaxEle, st.Loop
	r.StartLat, r.StartLng, r.EndLat, r.EndLng = nil, nil, nil, nil
	if n := len(r.Track); n > 0 {
		a, b := r.Track[0], r.Track[n-1]
		r.StartLat, r.StartLng, r.EndLat, r.EndLng = &a.Lat, &a.Lng, &b.Lat, &b.Lng
	}
	r.Preview = geo.Preview(r.Track, 80)
	r.Points = len(r.Track)
}

// placeWaypoint works out how far along its route a spot is (and its
// height when the spot has none).
func placeWaypoint(r *store.Route, w *store.Waypoint) {
	if len(r.Track) == 0 {
		return
	}
	at, _ := geo.Along(r.Track, w.Lat, w.Lng)
	w.AtM = math.Round(at)
	if w.Ele == nil {
		w.Ele = geo.EleAt(r.Track, at)
	}
}

// decorateRoute fills the numbers a page shows: the time it takes (the
// person's own, else worked out at their pace) and how hard it is.
func decorateRoute(r *store.Route, pace int) {
	r.Minutes = r.MinutesSet
	if r.Minutes <= 0 {
		r.Minutes = geo.Minutes(r.Activity, r.DistanceM, r.GainM, r.LossM, pace)
	}
	r.Level = r.Difficulty
	if r.Level == "" && r.DistanceM > 0 {
		r.Level = geo.Level(r.Activity, r.DistanceM, r.GainM)
	}
}

// decorateOuting fills an outing's route numbers (or what was walked).
func decorateOuting(o *store.Outing, rf store.RouteFacts, pace int) {
	if rf.MinutesSet > 0 {
		o.Minutes = rf.MinutesSet
	} else if o.DistanceM > 0 {
		o.Minutes = geo.Minutes(firstNonEmpty(rf.Activity, o.Activity), o.DistanceM, o.GainM, rf.LossM, pace)
	}
	if o.RouteID == nil && len(o.Actual) > 0 {
		var c geo.Comparison
		if err := jsonUnmarshal(o.Actual, &c); err == nil {
			o.DistanceM, o.GainM = c.DistanceM, c.GainM
			if c.MovingS > 0 {
				o.Minutes = int(c.TotalS / 60)
			}
		}
	}
}

// ---- outings ----

// startAt is when an outing starts (local), or the zero time.
func startAt(o *store.Outing) time.Time {
	hm := firstNonEmpty(o.StartTime, "09:00")
	t, err := time.ParseInLocation("2006-01-02 15:04", o.Date+" "+hm, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// backByTime is the back-by time (local), or the zero time.
func backByTime(o *store.Outing) time.Time {
	if o.BackBy == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", o.BackBy, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// suggestBackBy is the start plus the route's time, a fifth more for
// stops, and an hour, rounded up to the quarter hour.
func suggestBackBy(o *store.Outing) string {
	start := startAt(o)
	if start.IsZero() || o.Minutes <= 0 {
		return ""
	}
	m := float64(o.Minutes)*1.2 + 60
	t := start.Add(time.Duration(m) * time.Minute)
	if r := t.Minute() % 15; r != 0 {
		t = t.Add(time.Duration(15-r) * time.Minute)
	}
	return t.Format("2006-01-02T15:04")
}

// ---- words ----

func fmtDist(m float64, units string) string {
	if units == "imperial" {
		mi := m / 1609.344
		if mi < 10 {
			return fmt.Sprintf("%.1f mi", mi)
		}
		return fmt.Sprintf("%.0f mi", mi)
	}
	km := m / 1000
	if km < 10 {
		return fmt.Sprintf("%.1f km", km)
	}
	return fmt.Sprintf("%.0f km", km)
}

func fmtHeight(m float64, units string) string {
	if units == "imperial" {
		return fmt.Sprintf("%.0f ft", m*3.28084)
	}
	return fmt.Sprintf("%.0f m", m)
}

func fmtMinutes(m int) string {
	if m <= 0 {
		return "—"
	}
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %02d", m/60, m%60)
}

var activityWords = map[string]string{"hike": "Hike", "walk": "Walk", "run": "Run", "bike": "Ride", "gravel": "Gravel ride", "mtb": "Mountain-bike ride", "ski": "Ski tour", "other": "Outing"}

var levelWords = map[string]string{"easy": "easy", "moderate": "moderate", "hard": "hard", "expert": "very hard"}

var kindWords = map[string]string{"water": "Water", "hut": "Hut", "food": "Food", "shelter": "Shelter", "camp": "Camp", "summit": "Summit", "viewpoint": "Viewpoint",
	"lake": "Lake", "parking": "Parking", "transport": "Bus or train", "junction": "Junction", "danger": "Take care", "photo": "Photo spot", "other": "Spot"}

// healthKind is the Pocket Health workout kind of an activity.
func healthKind(activity string) string {
	switch activity {
	case "walk":
		return "walking"
	case "run":
		return "running"
	case "bike", "gravel", "mtb":
		return "cycling"
	case "ski":
		return "skiing"
	case "hike":
		return "hiking"
	}
	return "other"
}

func niceDate(d string) string {
	t, err := store.ParseDate(d)
	if err != nil {
		return d
	}
	return t.Format("Mon 2 Jan")
}

func whenWords(d int) string {
	switch {
	case d < -1:
		return fmt.Sprintf("%d days ago", -d)
	case d == -1:
		return "yesterday"
	case d == 0:
		return "today"
	case d == 1:
		return "tomorrow"
	}
	return fmt.Sprintf("in %d days", d)
}

func daysBetween(a, b string) int {
	ta, e1 := store.ParseDate(a)
	tb, e2 := store.ParseDate(b)
	if e1 != nil || e2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

func peopleNames(ps []store.Person) string {
	var n []string
	for _, p := range ps {
		if p.Name != "" {
			n = append(n, p.Name)
		} else if p.Email != "" {
			n = append(n, p.Email)
		}
	}
	return strings.Join(n, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
