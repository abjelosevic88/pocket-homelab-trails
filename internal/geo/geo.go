// Package geo is the arithmetic of a route: distances, the climb, how long
// it takes, how hard it is, where a spot is along it, sunrise and sunset,
// and how a walked track compares with the planned one. No network.
package geo

import (
	"math"
	"sort"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

const earthR = 6371008.8

func rad(d float64) float64 { return d * math.Pi / 180 }

// Dist is the great-circle distance in metres.
func Dist(lat1, lng1, lat2, lng2 float64) float64 {
	dLat, dLng := rad(lat2-lat1), rad(lng2-lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthR * math.Asin(math.Min(1, math.Sqrt(a)))
}

func d(a, b store.Pt) float64 { return Dist(a.Lat, a.Lng, b.Lat, b.Lng) }

// Cumulative is the distance from the start at each point.
func Cumulative(t []store.Pt) []float64 {
	out := make([]float64, len(t))
	for i := 1; i < len(t); i++ {
		out[i] = out[i-1] + d(t[i-1], t[i])
	}
	return out
}

// Stats is what a track adds up to.
type Stats struct {
	DistanceM float64
	GainM     float64
	LossM     float64
	MinEle    *float64
	MaxEle    *float64
	Loop      bool
}

// Measure adds up a track. The climb ignores wobbles under 4 m (GPS and
// height-model noise would otherwise double it).
func Measure(t []store.Pt) Stats {
	var s Stats
	if len(t) == 0 {
		return s
	}
	cum := Cumulative(t)
	s.DistanceM = cum[len(cum)-1]
	s.GainM, s.LossM = climb(t, 4)
	for _, p := range t {
		if p.Ele == nil {
			continue
		}
		v := *p.Ele
		if s.MinEle == nil || v < *s.MinEle {
			s.MinEle = ptr(v)
		}
		if s.MaxEle == nil || v > *s.MaxEle {
			s.MaxEle = ptr(v)
		}
	}
	s.Loop = len(t) > 2 && s.DistanceM > 500 && d(t[0], t[len(t)-1]) < math.Max(150, s.DistanceM*0.02)
	if s.Loop && retraces(t, cum) {
		s.Loop = false // there and back the same way is not a loop
	}
	return s
}

func ptr(v float64) *float64 { return &v }

// retraces reports whether the way back is the way out: the points a
// quarter and three quarters along (and a tenth and nine tenths) lie
// close together.
func retraces(t []store.Pt, cum []float64) bool {
	total := cum[len(cum)-1]
	at := func(f float64) store.Pt {
		i := sort.SearchFloat64s(cum, total*f)
		return t[min(i, len(t)-1)]
	}
	near := math.Max(120, total*0.02)
	return d(at(.25), at(.75)) < near && d(at(.1), at(.9)) < near
}

// climb counts the ups and downs that are more than the threshold.
func climb(t []store.Pt, threshold float64) (gain, loss float64) {
	ref := math.NaN()
	for _, p := range t {
		if p.Ele == nil {
			continue
		}
		v := *p.Ele
		if math.IsNaN(ref) {
			ref = v
			continue
		}
		switch diff := v - ref; {
		case diff >= threshold:
			gain += diff
			ref = v
		case diff <= -threshold:
			loss -= diff
			ref = v
		}
	}
	return gain, loss
}

// HasEle reports whether most points have a height.
func HasEle(t []store.Pt) bool {
	n := 0
	for _, p := range t {
		if p.Ele != nil {
			n++
		}
	}
	return len(t) > 0 && n*2 >= len(t)
}

// ---- how long and how hard ----

// Minutes is the usual time for a route, at pace percent of the usual
// speed (100 = the usual; 120 = a fifth faster). Walking times follow the
// hiking clubs' rule (DIN 33466): 4 km an hour on the flat, 300 m an hour
// up and 500 m down, the larger of flat and height time plus half the
// smaller. Rides: an average speed and the climbing on top.
func Minutes(activity string, distM, gainM, lossM float64, pace int) int {
	if distM <= 0 {
		return 0
	}
	if pace <= 0 {
		pace = 100
	}
	km := distM / 1000
	var h float64
	walk := func(kmh, up, down float64) float64 {
		flat := km / kmh
		vert := gainM/up + lossM/down
		return math.Max(flat, vert) + math.Min(flat, vert)/2
	}
	switch activity {
	case "walk":
		h = walk(4.5, 350, 550)
	case "run":
		h = walk(9, 700, 1100)
	case "ski":
		h = walk(4.5, 350, 1500)
	case "bike":
		h = km/22 + gainM/700
	case "gravel":
		h = km/18 + gainM/550
	case "mtb":
		h = km/12 + gainM/450
	default: // hike and the rest
		h = walk(4, 300, 500)
	}
	m := h * 60 * 100 / float64(pace)
	return int(math.Round(m/5) * 5) // nobody plans to the minute
}

// Level is how hard a route is, from its length and its climb: easy,
// moderate, hard or expert. Rides count about three times the distance.
func Level(activity string, distM, gainM float64) string {
	km := distM / 1000
	switch activity {
	case "bike", "gravel":
		km /= 3.5
		gainM /= 1.6
	case "mtb":
		km /= 2.2
		gainM /= 1.3
	case "run":
		km /= 1.4
	}
	switch {
	case km <= 8 && gainM <= 350:
		return "easy"
	case km <= 16 && gainM <= 900:
		return "moderate"
	case km <= 25 && gainM <= 1500:
		return "hard"
	}
	return "expert"
}

// ---- thinning ----

// Simplify keeps the shape of a track within tol metres (Douglas–Peucker);
// heights and times ride along with the points kept.
func Simplify(t []store.Pt, tol float64) []store.Pt {
	if len(t) < 3 {
		return t
	}
	keep := make([]bool, len(t))
	keep[0], keep[len(t)-1] = true, true
	type span struct{ a, b int }
	stack := []span{{0, len(t) - 1}}
	for len(stack) > 0 {
		sp := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		best, at := 0.0, -1
		for i := sp.a + 1; i < sp.b; i++ {
			if v := segDist(t[i], t[sp.a], t[sp.b]); v > best {
				best, at = v, i
			}
		}
		if at >= 0 && best > tol {
			keep[at] = true
			stack = append(stack, span{sp.a, at}, span{at, sp.b})
		}
	}
	out := make([]store.Pt, 0, len(t)/2)
	for i, k := range keep {
		if k {
			out = append(out, t[i])
		}
	}
	return out
}

// Thin simplifies until at most max points are left.
func Thin(t []store.Pt, max int) []store.Pt {
	if len(t) <= max {
		return t
	}
	for tol := 2.0; ; tol *= 1.6 {
		s := Simplify(t, tol)
		if len(s) <= max || tol > 5000 {
			return s
		}
	}
}

// Preview is a thin line for lists and the overview map: [[lat, lng], …].
func Preview(t []store.Pt, max int) [][]float64 {
	s := Thin(t, max)
	out := make([][]float64, len(s))
	for i, p := range s {
		out[i] = []float64{math.Round(p.Lat*1e5) / 1e5, math.Round(p.Lng*1e5) / 1e5}
	}
	return out
}

// segDist is the distance in metres from p to the segment a–b, on a flat
// projection around a (fine at the scale of a walk).
func segDist(p, a, b store.Pt) float64 {
	px, py := project(p, a)
	bx, by := project(b, a)
	l2 := bx*bx + by*by
	if l2 == 0 {
		return math.Hypot(px, py)
	}
	u := math.Max(0, math.Min(1, (px*bx+py*by)/l2))
	return math.Hypot(px-u*bx, py-u*by)
}

func project(p, origin store.Pt) (x, y float64) {
	x = rad(p.Lng-origin.Lng) * math.Cos(rad(origin.Lat)) * earthR
	y = rad(p.Lat-origin.Lat) * earthR
	return
}

// ---- along a route ----

// Along says where a spot lies along a track: the distance from the start
// of the nearest part, and how far off the track it is.
func Along(t []store.Pt, lat, lng float64) (atM, offM float64) {
	if len(t) == 0 {
		return 0, math.Inf(1)
	}
	if len(t) == 1 {
		return 0, Dist(lat, lng, t[0].Lat, t[0].Lng)
	}
	p := store.Pt{Lat: lat, Lng: lng}
	cum := Cumulative(t)
	offM = math.Inf(1)
	type hit struct{ at, off float64 }
	var hits []hit
	for i := 1; i < len(t); i++ {
		a, b := t[i-1], t[i]
		px, py := project(p, a)
		bx, by := project(b, a)
		l2 := bx*bx + by*by
		u := 0.0
		if l2 > 0 {
			u = math.Max(0, math.Min(1, (px*bx+py*by)/l2))
		}
		off := math.Hypot(px-u*bx, py-u*by)
		hits = append(hits, hit{cum[i-1] + u*(cum[i]-cum[i-1]), off})
		offM = math.Min(offM, off)
	}
	// a way walked twice (there and back): the first pass counts
	for _, h := range hits {
		if h.off <= offM+25 {
			return h.at, offM
		}
	}
	return 0, offM
}

// EleAt is the height at a distance along a track (nil when unknown).
func EleAt(t []store.Pt, atM float64) *float64 {
	cum := Cumulative(t)
	i := sort.SearchFloat64s(cum, atM)
	for j := i; j >= 0 && j < len(t); j-- {
		if t[j].Ele != nil {
			return ptr(*t[j].Ele)
		}
	}
	return nil
}

// ---- planned against walked ----

// Comparison is how a walked track went against the planned route.
type Comparison struct {
	DistanceM  float64 `json:"distance_m"`
	GainM      float64 `json:"gain_m"`
	TotalS     int64   `json:"total_s"`  // from setting off to arriving
	MovingS    int64   `json:"moving_s"` // without the stops
	StoppedS   int64   `json:"stopped_s"`
	From       int64   `json:"from"`
	To         int64   `json:"to"`
	Points     int     `json:"points"`
	CoveredPct int     `json:"covered_pct"` // of the planned route walked (within 120 m)
	MaxOffM    float64 `json:"max_off_m"`   // furthest from the planned route
	OffKm      float64 `json:"off_km"`      // walked more than 150 m off it
	PlannedM   float64 `json:"planned_m"`
	PlannedMin int     `json:"planned_min"`
	SpeedPct   int     `json:"speed_pct"` // your moving speed against the usual time (100 = as expected)
	HasEle     bool    `json:"has_ele"`
}

// Trim cuts a day of recorded points to the outing: from the first point
// near the route to the last one near it (within near metres). With no
// route the whole day stays.
func Trim(route, day []store.Pt, near float64) []store.Pt {
	if len(route) < 2 || len(day) == 0 {
		return day
	}
	first, last := -1, -1
	for i, p := range day {
		if _, off := Along(route, p.Lat, p.Lng); off <= near {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return nil
	}
	return day[first : last+1]
}

// Clean drops jumps a body cannot make (a bad fix 2 km away for a second)
// and repeats of the same spot.
func Clean(t []store.Pt) []store.Pt {
	if len(t) < 2 {
		return t
	}
	out := []store.Pt{t[0]}
	for _, p := range t[1:] {
		prev := out[len(out)-1]
		dist := d(prev, p)
		if dist < 1 {
			continue
		}
		if p.T > 0 && prev.T > 0 {
			dt := float64(p.T - prev.T)
			if dt < 0 || (dt > 0 && dist/dt > 40 && dist > 200) { // back in time, or faster than 144 km/h
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// Compare measures a walked track against the planned route (route may be
// empty: then only the walked numbers are filled in).
func Compare(route, walked []store.Pt, activity string, pace int) Comparison {
	var c Comparison
	walked = Clean(walked)
	c.Points = len(walked)
	if len(walked) < 2 {
		return c
	}
	st := Measure(walked)
	c.DistanceM, c.GainM, c.HasEle = math.Round(st.DistanceM), math.Round(st.GainM), HasEle(walked)
	c.From, c.To = walked[0].T, walked[len(walked)-1].T
	if c.To > c.From {
		c.TotalS = c.To - c.From
	}
	for i := 1; i < len(walked); i++ {
		a, b := walked[i-1], walked[i]
		dt := b.T - a.T
		if dt <= 0 {
			continue
		}
		// moving: faster than a slow shuffle, and no gap longer than ten minutes
		if dt <= 600 && d(a, b)/float64(dt) >= 0.3 {
			c.MovingS += dt
		}
	}
	c.StoppedS = max(c.TotalS-c.MovingS, 0)
	if len(route) >= 2 {
		rs := Measure(route)
		c.PlannedM = math.Round(rs.DistanceM)
		c.PlannedMin = Minutes(activity, rs.DistanceM, rs.GainM, rs.LossM, 100)
		for i, p := range walked {
			_, off := Along(route, p.Lat, p.Lng)
			c.MaxOffM = math.Max(c.MaxOffM, off)
			if i > 0 && off > 150 {
				c.OffKm += d(walked[i-1], p) / 1000
			}
		}
		c.MaxOffM, c.OffKm = math.Round(c.MaxOffM), math.Round(c.OffKm*10)/10
		// sample the route every 50 m: how much of it has walked points near it
		samples, covered := 0, 0
		cum := Cumulative(route)
		next := 0.0
		for i, p := range route {
			if cum[i] < next {
				continue
			}
			next = cum[i] + 50
			samples++
			if _, off := Along(walked, p.Lat, p.Lng); off <= 120 {
				covered++
			}
		}
		if samples > 0 {
			c.CoveredPct = covered * 100 / samples
		}
	}
	if c.MovingS > 0 {
		usual := Minutes(activity, st.DistanceM, st.GainM, st.LossM, 100)
		if usual > 0 {
			c.SpeedPct = int(math.Round(float64(usual*60) / float64(c.MovingS) * 100))
		}
	}
	_ = pace
	return c
}

// ---- the sun ----

// Sun gives sunrise and sunset (local to loc) at a spot on a day, NOAA's
// approximation; ok is false in polar day or night.
func Sun(date time.Time, lat, lng float64, loc *time.Location) (rise, set time.Time, ok bool) {
	y, m, dd := date.Date()
	noon := time.Date(y, m, dd, 12, 0, 0, 0, time.UTC)
	jd := float64(noon.Unix())/86400 + 2440587.5
	n := math.Floor(jd - 2451545.0 + 0.0008)
	jStar := n - lng/360
	mAnom := math.Mod(357.5291+0.98560028*jStar, 360)
	mr := rad(mAnom)
	cEq := 1.9148*math.Sin(mr) + 0.0200*math.Sin(2*mr) + 0.0003*math.Sin(3*mr)
	lambda := math.Mod(mAnom+cEq+180+102.9372, 360)
	lr := rad(lambda)
	jTransit := 2451545.0 + jStar + 0.0053*math.Sin(mr) - 0.0069*math.Sin(2*lr)
	decl := math.Asin(math.Sin(lr) * math.Sin(rad(23.4397)))
	cosH := (math.Sin(rad(-0.833)) - math.Sin(rad(lat))*math.Sin(decl)) / (math.Cos(rad(lat)) * math.Cos(decl))
	if cosH < -1 || cosH > 1 {
		return time.Time{}, time.Time{}, false
	}
	h := math.Acos(cosH) * 180 / math.Pi
	toTime := func(j float64) time.Time {
		sec := (j - 2440587.5) * 86400
		return time.Unix(int64(math.Round(sec)), 0).In(loc)
	}
	return toTime(jTransit - h/360), toTime(jTransit + h/360), true
}
