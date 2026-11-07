package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-trails/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// demoRoute is a route around Sarajevo, drawn through these points
// (lat, lng, height); the demo adds the wiggles of a real path.
type demoRoute struct {
	name, activity, region, start, desc string
	via                                 [][3]float64
	loop, back                          bool // back: there and back the same way
	wps                                 []demoWP
	fav                                 bool
	tags                                []string
}

type demoWP struct {
	kind, name, note string
	at               int // index into via
}

var demoRoutes = []demoRoute{
	{name: "Trebević from the cable car", activity: "hike", region: "Trebević, Sarajevo", start: "Upper cable car station", fav: true, tags: []string{"close by", "views"},
		desc: "From the top of the cable car up through the beech woods to Đokin toranj, back past the old bobsleigh track.",
		via:  [][3]float64{{43.8467, 18.4486, 1160}, {43.8421, 18.4529, 1230}, {43.8372, 18.4561, 1330}, {43.8318, 18.4569, 1450}, {43.8256, 18.4524, 1627}, {43.8301, 18.4470, 1480}, {43.8366, 18.4431, 1320}, {43.8419, 18.4455, 1210}, {43.8467, 18.4486, 1160}},
		loop: true, wps: []demoWP{{"summit", "Đokin toranj", "The tower on top: views over the whole city.", 4}, {"viewpoint", "Bobsleigh track", "Graffiti and the old Olympic track.", 6}, {"food", "Kafana on Trebević", "Coffee and pita after.", 0}}},
	{name: "Bjelašnica summit", activity: "hike", region: "Bjelašnica", start: "Babin Do car park", tags: []string{"summit", "2000 m"},
		desc: "Straight up from the ski centre to the weather station on top. Exposed: no shade, wind on the ridge.",
		via:  [][3]float64{{43.7372, 18.2614, 1270}, {43.7311, 18.2601, 1420}, {43.7254, 18.2588, 1610}, {43.7192, 18.2570, 1830}, {43.7116, 18.2551, 2067}},
		back: true, wps: []demoWP{{"parking", "Babin Do", "", 0}, {"water", "Spring under the lift", "Usually running until August.", 1}, {"summit", "Bjelašnica (2067 m)", "Weather station; wind shelter behind it.", 4}, {"danger", "Open ridge", "Strong wind, no shelter.", 3}}},
	{name: "Umoljani to Lukomir", activity: "hike", region: "Bjelašnica", start: "Umoljani", fav: true, tags: []string{"villages", "canyon"},
		desc: "Across the high plateau above the Rakitnica canyon to the highest village in the country.",
		via:  [][3]float64{{43.6623, 18.2316, 1340}, {43.6581, 18.2232, 1420}, {43.6532, 18.2141, 1490}, {43.6490, 18.2060, 1520}, {43.6448, 18.1983, 1470}},
		back: true, wps: []demoWP{{"food", "Umoljani", "Pita and kajmak in the village.", 0}, {"viewpoint", "Rakitnica canyon edge", "Stay back from the edge.", 3}, {"hut", "Lukomir", "Coffee at the house with the blue door.", 4}, {"water", "Well in Lukomir", "", 4}}},
	{name: "Skakavac waterfall", activity: "hike", region: "Nahorevo, Sarajevo", start: "Skakavac hut road", tags: []string{"waterfall", "easy"},
		desc: "A short walk through the woods to the 98 m waterfall. Slippery near the falls after rain.",
		via:  [][3]float64{{43.9356, 18.4505, 1020}, {43.9398, 18.4471, 1080}, {43.9441, 18.4425, 1130}, {43.9483, 18.4380, 1010}},
		back: true, wps: []demoWP{{"hut", "Skakavac mountain hut", "Open at weekends.", 0}, {"viewpoint", "The falls", "", 3}, {"danger", "Wet rocks", "", 3}}},
	{name: "Ilidža to Vrelo Bosne", activity: "walk", region: "Ilidža", start: "Ilidža tram stop", tags: []string{"flat", "family"},
		desc: "Down the plane-tree avenue to the springs of the Bosna, horse carriages and swans.",
		via:  [][3]float64{{43.8299, 18.3101, 500}, {43.8262, 18.2967, 502}, {43.8226, 18.2831, 504}, {43.8186, 18.2690, 505}},
		back: true, wps: []demoWP{{"transport", "Ilidža tram", "Tram 3 from town.", 0}, {"lake", "Vrelo Bosne", "The springs.", 3}, {"food", "Café at the springs", "", 3}}},
	{name: "Jahorina: Ogorjelica", activity: "hike", region: "Jahorina", start: "Poljice", tags: []string{"summit"},
		desc: "Up the ski slopes to the second-highest top of Jahorina; big views to the south.",
		via:  [][3]float64{{43.7330, 18.5660, 1560}, {43.7290, 18.5722, 1660}, {43.7248, 18.5790, 1780}, {43.7192, 18.5876, 1916}, {43.7243, 18.5841, 1800}, {43.7301, 18.5731, 1620}, {43.7330, 18.5660, 1560}},
		loop: true, wps: []demoWP{{"summit", "Ogorjelica (1916 m)", "", 3}, {"hut", "Hotel Rajska vrata", "Hot chocolate.", 0}}},
	{name: "Sarajevo – Vrelo Bosne loop", activity: "bike", region: "Sarajevo", start: "Marijin Dvor", tags: []string{"flat", "after work"},
		desc: "Along the river to Ilidža and the springs, back through Butmir.",
		via:  [][3]float64{{43.8569, 18.4063, 520}, {43.8522, 18.3780, 515}, {43.8436, 18.3450, 505}, {43.8299, 18.3101, 500}, {43.8186, 18.2690, 505}, {43.8168, 18.3010, 505}, {43.8268, 18.3390, 510}, {43.8455, 18.3810, 515}, {43.8569, 18.4063, 520}},
		loop: true, wps: []demoWP{{"water", "Fountain at Vrelo Bosne", "", 4}}},
	{name: "Igman by gravel", activity: "gravel", region: "Igman", start: "Hrasnica", tags: []string{"climb"},
		desc: "The old Olympic road up to Veliko polje and the ski jumps, down the forest track.",
		via:  [][3]float64{{43.7992, 18.3165, 520}, {43.7860, 18.3060, 760}, {43.7730, 18.2910, 980}, {43.7555, 18.2662, 1250}, {43.7640, 18.2850, 1150}, {43.7800, 18.3000, 850}, {43.7992, 18.3165, 520}},
		loop: true, wps: []demoWP{{"viewpoint", "Olympic ski jumps", "", 3}, {"food", "Veliko polje", "Grill on the meadow.", 3}}},
	{name: "Wilson's Promenade run", activity: "run", region: "Sarajevo", start: "Wilsonovo šetalište", tags: []string{"flat"},
		desc: "Along the river under the plane trees and back.",
		via:  [][3]float64{{43.8552, 18.3947, 518}, {43.8540, 18.3800, 516}, {43.8520, 18.3640, 514}},
		back: true},
}

// buildTrack walks through the points with a path's wiggles every ~35 m;
// heights follow the points with a little noise.
func buildTrack(rnd *rand.Rand, via [][3]float64, back bool) []store.Pt {
	var out []store.Pt
	add := func(a, b [3]float64) {
		d := geo.Dist(a[0], a[1], b[0], b[1])
		n := max(int(d/35), 1)
		steep := math.Abs(b[2]-a[2]) / math.Max(d, 1)
		amp := 0.00012 + steep*0.0012 // switchbacks on the steep bits
		phase := rnd.Float64() * math.Pi
		for k := 0; k < n; k++ {
			f := float64(k) / float64(n)
			lat := a[0] + (b[0]-a[0])*f
			lng := a[1] + (b[1]-a[1])*f
			w := math.Sin(f*math.Pi*float64(2+rnd.Intn(2))+phase) * amp * math.Sin(f*math.Pi)
			// sideways to the direction of travel
			dx, dy := b[1]-a[1], b[0]-a[0]
			l := math.Hypot(dx, dy)
			if l > 0 {
				lat += -dx / l * w
				lng += dy / l * w
			}
			e := a[2] + (b[2]-a[2])*(f*f*(3-2*f)) + rnd.NormFloat64()*1.2
			out = append(out, store.Pt{Lat: lat, Lng: lng, Ele: &e})
		}
	}
	for i := 1; i < len(via); i++ {
		add(via[i-1], via[i])
	}
	last := via[len(via)-1]
	e := last[2]
	out = append(out, store.Pt{Lat: last[0], Lng: last[1], Ele: &e})
	if back {
		n := len(out)
		for i := n - 2; i >= 0; i-- {
			p := out[i]
			p.Lat += 0.00003
			ee := *p.Ele + rnd.NormFloat64()
			p.Ele = &ee
			out = append(out, p)
		}
	}
	return out
}

// walkIt is a recorded walk of a route: GPS jitter, a pace, a few stops.
func walkIt(rnd *rand.Rand, track []store.Pt, start time.Time, kmh float64) []store.Pt {
	var out []store.Pt
	t := start
	cum := geo.Cumulative(track)
	stopAt := map[int]int{}
	for k := 0; k < 2; k++ {
		stopAt[len(track)/3+rnd.Intn(max(len(track)/3, 1))] = 300 + rnd.Intn(900)
	}
	for i, p := range track {
		if i > 0 {
			d := cum[i] - cum[i-1]
			climb := 0.0
			if p.Ele != nil && track[i-1].Ele != nil {
				climb = math.Max(0, *p.Ele-*track[i-1].Ele)
			}
			sec := d/(kmh/3.6) + climb*6 // about 600 m an hour up
			t = t.Add(time.Duration(sec*(0.9+rnd.Float64()*0.2)) * time.Second)
		}
		if s, ok := stopAt[i]; ok {
			t = t.Add(time.Duration(s) * time.Second)
		}
		q := store.Pt{Lat: p.Lat + rnd.NormFloat64()*0.00004, Lng: p.Lng + rnd.NormFloat64()*0.00005, T: t.Unix()}
		if p.Ele != nil {
			e := *p.Ele + rnd.NormFloat64()*3
			q.Ele = &e
		}
		out = append(out, q)
	}
	return out
}

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if have, err := s.store.ListRoutes(ctx, uid, store.RouteFilter{}); err != nil || len(have) > 0 {
		return err
	}
	rnd := rand.New(rand.NewSource(42))
	st := s.settingsOf(ctx, uid)
	st.ContactName, st.ContactEmail = "Lejla Hadžić", "lejla@example.com"
	st.SafetyDefault = true
	_ = s.store.SaveUserSettings(ctx, uid, st)

	// ---- routes ----
	var routes []*store.Route
	for _, d := range demoRoutes {
		rt := &store.Route{Name: d.name, Activity: d.activity, Region: d.region, StartName: d.start, Description: d.desc, Tags: d.tags, Favourite: d.fav,
			Track: buildTrack(rnd, d.via, d.back), Source: "demo", SourceRef: d.name}
		prepRoute(rt)
		if err := s.store.CreateRoute(ctx, uid, rt); err != nil {
			return err
		}
		for _, w := range d.wps {
			v := d.via[w.at]
			e := v[2]
			wp := &store.Waypoint{RouteID: rt.ID, Kind: w.kind, Name: w.name, Note: w.note, Lat: v[0] + 0.0002, Lng: v[1] + 0.0002, Ele: &e}
			placeWaypoint(rt, wp)
			if err := s.store.CreateWaypoint(ctx, uid, wp); err != nil {
				return err
			}
		}
		routes = append(routes, rt)
	}

	// ---- gear ----
	type g struct {
		name, cat string
		w         int
		worn      bool
		cons      bool
	}
	gearList := []g{
		{"Daypack 24 l", "pack", 890, false, false}, {"Rain cover", "pack", 85, false, false},
		{"Hiking boots", "footwear", 1240, true, false}, {"Trail runners", "footwear", 620, true, false}, {"Merino socks", "footwear", 70, true, false},
		{"Rain jacket", "clothing", 310, false, false}, {"Fleece mid-layer", "clothing", 380, false, false}, {"Down jacket", "clothing", 290, false, false},
		{"Sun hat", "clothing", 70, true, false}, {"Warm hat and gloves", "clothing", 120, false, false}, {"Hiking trousers", "clothing", 360, true, false},
		{"Water bottle 1 l", "water", 160, false, false}, {"Water (1.5 l)", "water", 1500, false, true}, {"Water filter", "water", 85, false, false},
		{"Sandwiches", "food", 350, false, true}, {"Snack bars", "food", 180, false, true}, {"Thermos with tea", "cook", 520, false, false},
		{"Paper map", "navigation", 90, false, false}, {"Compass", "navigation", 40, false, false},
		{"Head torch", "safety", 75, false, false}, {"Emergency blanket", "safety", 60, false, false}, {"Whistle", "safety", 10, false, false}, {"Knife", "safety", 85, false, false},
		{"First-aid kit", "first_aid", 230, false, false}, {"Blister plasters", "first_aid", 20, false, false},
		{"Phone", "electronics", 200, true, false}, {"Power bank", "electronics", 190, false, false},
		{"Sunglasses", "clothing", 30, true, false}, {"Sun cream", "first_aid", 60, false, true},
		{"Trekking poles", "other", 480, false, false}, {"Microspikes", "snow", 390, false, false},
		{"Bike helmet", "bike", 280, true, false}, {"Spare tube and pump", "bike", 240, false, false}, {"Bike lights", "bike", 150, false, false},
	}
	gearIDs := map[string]int64{}
	for _, x := range gearList {
		gr := &store.Gear{Name: x.name, Category: x.cat, WeightG: x.w, Worn: x.worn, Consumable: x.cons}
		if err := s.store.CreateGear(ctx, uid, gr); err != nil {
			return err
		}
		gearIDs[x.name] = gr.ID
	}
	pack := func(name, activity string, names ...string) int64 {
		p := &store.Pack{Name: name, Activity: activity}
		if err := s.store.CreatePack(ctx, uid, p); err != nil {
			return 0
		}
		var items []store.PackItem
		for _, n := range names {
			items = append(items, store.PackItem{GearID: gearIDs[n], Qty: 1})
		}
		_ = s.store.SetPackItems(ctx, uid, p.ID, items)
		return p.ID
	}
	dayHike := pack("Day hike", "hike", "Daypack 24 l", "Hiking boots", "Merino socks", "Rain jacket", "Fleece mid-layer", "Sun hat", "Hiking trousers", "Water bottle 1 l",
		"Water (1.5 l)", "Sandwiches", "Snack bars", "Paper map", "Head torch", "First-aid kit", "Phone", "Power bank", "Sunglasses", "Sun cream", "Whistle")
	pack("Winter day", "hike", "Daypack 24 l", "Hiking boots", "Merino socks", "Rain jacket", "Fleece mid-layer", "Down jacket", "Warm hat and gloves", "Thermos with tea",
		"Snack bars", "Head torch", "Emergency blanket", "First-aid kit", "Phone", "Power bank", "Microspikes", "Trekking poles")
	pack("Bike day", "bike", "Bike helmet", "Spare tube and pump", "Bike lights", "Water bottle 1 l", "Snack bars", "Rain jacket", "Phone", "Sunglasses")

	// ---- outings: eight months behind, two ahead ----
	people := [][]store.Person{{{Name: "Amar Kovač", Email: "amar@example.com"}}, {{Name: "Lejla Hadžić", Email: "lejla@example.com"}}, {{Name: "Amar Kovač"}, {Name: "Mirza Begić"}}, nil}
	conds := []string{"Dry and firm all the way.", "Muddy in the woods after the rain; poles helped.", "Snow from 1,600 m, microspikes needed on the top part.",
		"Busy at the top at noon; quiet before ten.", "The spring was dry — bring all your water.", "Path well marked, a fallen tree past the first bend.", ""}
	t0 := time.Now()
	day := func(back int) time.Time {
		d := t0.AddDate(0, 0, -back)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
	}
	backs := []int{3, 9, 13, 20, 27, 34, 41, 48, 55, 62, 70, 77, 90, 104, 118, 131, 146, 160, 175, 189, 203, 217, 231, 240}
	for i, b := range backs {
		rt := routes[rnd.Intn(len(routes))]
		if i%4 == 0 {
			rt = routes[i%3]
		}
		d := day(b)
		hour := 8 + rnd.Intn(3)
		start := d.Add(time.Duration(hour)*time.Hour + time.Duration(rnd.Intn(4)*15)*time.Minute)
		kmh := map[string]float64{"hike": 4.2, "walk": 4.6, "run": 10, "bike": 22, "gravel": 16}[rt.Activity]
		pts := walkIt(rnd, rt.Track, start, kmh)
		o := &store.Outing{RouteID: &rt.ID, Activity: rt.Activity, Date: d.Format("2006-01-02"), StartTime: start.Format("15:04"), Status: store.OutingDone,
			People: people[rnd.Intn(len(people))], Rating: 3 + rnd.Intn(3), Conditions: conds[rnd.Intn(len(conds))], AlertAfter: 60, Source: "demo", SourceRef: fmt.Sprint("o", i),
			StartedAt: time.Unix(pts[0].T, 0).UTC().Format(time.RFC3339), EndedAt: time.Unix(pts[len(pts)-1].T, 0).UTC().Format(time.RFC3339)}
		if i == 0 {
			o.Rating = 0 // waits for "how did it go?"
			o.Conditions = ""
		}
		if err := s.store.CreateOuting(ctx, uid, o); err != nil {
			return err
		}
		cmp := geo.Compare(rt.Track, pts, rt.Activity, 100)
		decorateRoute(rt, 100)
		cmp.PlannedMin = rt.Minutes
		o.UserID, o.ActualTrack, o.ActualSource = uid, geo.Thin(pts, maxTrack), "movement"
		b, _ := json.Marshal(cmp)
		o.Actual = b
		_ = s.store.SetOutingActual(ctx, o)
	}
	// planned yesterday, never marked
	{
		rt := routes[3]
		o := &store.Outing{RouteID: &rt.ID, Activity: rt.Activity, Date: day(1).Format("2006-01-02"), StartTime: "10:00", Status: store.OutingPlanned, People: people[0],
			AlertAfter: 60, Source: "demo", SourceRef: "unmarked"}
		_ = s.store.CreateOuting(ctx, uid, o)
	}
	// the next one: in two days, Bjelašnica, packing half done, someone to tell
	next := day(-2)
	weather := func(date string, cond string, code int, tmin, tmax, prob, gust float64) json.RawMessage {
		w := dayWeather{Date: date, Code: code, Condition: cond, TMax: &tmax, TMin: &tmin, PrecipProb: &prob, Gust: &gust, Source: "demo", FetchedAt: time.Now().UTC().Format(time.RFC3339)}
		for h := 6; h <= 20; h++ {
			tt := tmin + (tmax-tmin)*math.Sin(math.Pi*float64(h-6)/14)
			pp := prob * math.Max(0, math.Sin(math.Pi*float64(h-8)/12))
			gg := gust * (0.6 + 0.4*math.Sin(math.Pi*float64(h-6)/14))
			w.Hours = append(w.Hours, hourWeather{Time: fmt.Sprintf("%02d:00", h), T: &tt, PrecipProb: &pp, Code: code, Gust: &gg})
		}
		b, _ := json.Marshal(w)
		return b
	}
	{
		rt := routes[1]
		o := &store.Outing{RouteID: &rt.ID, Activity: rt.Activity, Date: next.Format("2006-01-02"), StartTime: "08:30", Status: store.OutingPlanned,
			People: []store.Person{{Name: "Amar Kovač", Email: "amar@example.com"}}, Notes: "Meet at the petrol station in Hrasnica at 7:45.",
			Safety: true, ContactName: "Lejla Hadžić", ContactEmail: "lejla@example.com", AlertAfter: 60, BackBy: next.Format("2006-01-02") + "T16:30",
			Source: "demo", SourceRef: "next"}
		o.Weather = weather(o.Date, "Partly cloudy", 2, 1, 9, 30, 52)
		if err := s.store.CreateOuting(ctx, uid, o); err != nil {
			return err
		}
		_ = s.addPack(ctx, uid, o.ID, dayHike)
		items, _ := s.store.ListOutingItems(ctx, uid, o.ID)
		for i, it := range items {
			if i%2 == 0 {
				it.Packed = true
				_ = s.store.UpdateOutingItem(ctx, it)
			}
		}
		if len(items) > 3 {
			items[3].ToBuy = true
			items[3].Packed = false
			_ = s.store.UpdateOutingItem(ctx, items[3])
		}
	}
	{
		rt := routes[2]
		d := day(-9)
		o := &store.Outing{RouteID: &rt.ID, Activity: rt.Activity, Date: d.Format("2006-01-02"), StartTime: "09:00", Status: store.OutingPlanned,
			People: people[2], AlertAfter: 60, Source: "demo", SourceRef: "later", Notes: "Lunch in Lukomir."}
		_ = s.store.CreateOuting(ctx, uid, o)
	}
	s.log.Info("demo data seeded", "routes", len(routes), "outings", len(backs)+3)
	return nil
}
