package store

import (
	"encoding/json"
	"math"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Kinds and states (the API checks against these).
var (
	Activities    = []string{"hike", "walk", "run", "bike", "gravel", "mtb", "ski", "other"}
	Difficulties  = []string{"", "easy", "moderate", "hard", "expert"}
	WaypointKinds = []string{"water", "hut", "food", "shelter", "camp", "summit", "viewpoint", "lake", "parking", "transport", "junction", "danger", "photo", "other"}
	GearKinds     = []string{"pack", "shelter", "sleep", "clothing", "footwear", "cook", "water", "food", "navigation", "safety", "first_aid", "electronics", "bike", "snow", "other"}
	OutingStates  = []string{"planned", "out", "done", "cancelled"}
)

// Outing states.
const (
	OutingPlanned   = "planned"
	OutingOut       = "out"
	OutingDone      = "done"
	OutingCancelled = "cancelled"
)

// Pt is one point of a track: latitude, longitude, height (nil when the
// file had none) and, for a recorded track, the time (unix seconds).
type Pt struct {
	Lat float64
	Lng float64
	Ele *float64
	T   int64
}

// MarshalJSON writes a point as [lat, lng, ele] or [lat, lng, ele, t].
func (p Pt) MarshalJSON() ([]byte, error) {
	var ele any
	if p.Ele != nil {
		ele = math.Round(*p.Ele*10) / 10
	}
	if p.T != 0 {
		return json.Marshal([]any{round6(p.Lat), round6(p.Lng), ele, p.T})
	}
	return json.Marshal([]any{round6(p.Lat), round6(p.Lng), ele})
}

// UnmarshalJSON reads [lat, lng], [lat, lng, ele] or [lat, lng, ele, t].
func (p *Pt) UnmarshalJSON(b []byte) error {
	var a []*float64
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) < 2 || a[0] == nil || a[1] == nil {
		return nil
	}
	p.Lat, p.Lng = *a[0], *a[1]
	if len(a) > 2 && a[2] != nil {
		v := *a[2]
		p.Ele = &v
	}
	if len(a) > 3 && a[3] != nil {
		p.T = int64(*a[3])
	}
	return nil
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// Route is a hike, a walk, a run or a ride.
type Route struct {
	ID          int64       `json:"id"`
	UserID      int64       `json:"-"`
	Name        string      `json:"name"`
	Activity    string      `json:"activity"`
	Difficulty  string      `json:"difficulty"` // as set ("" = worked out)
	Region      string      `json:"region"`
	Description string      `json:"description"`
	Tags        []string    `json:"tags"`
	Track       []Pt        `json:"track,omitempty"`
	Controls    [][]float64 `json:"controls,omitempty"`
	Drawn       string      `json:"drawn"`
	DistanceM   float64     `json:"distance_m"`
	GainM       float64     `json:"gain_m"`
	LossM       float64     `json:"loss_m"`
	MinEle      *float64    `json:"min_ele"`
	MaxEle      *float64    `json:"max_ele"`
	MinutesSet  int         `json:"minutes_set"`
	Loop        bool        `json:"loop"`
	StartLat    *float64    `json:"start_lat"`
	StartLng    *float64    `json:"start_lng"`
	EndLat      *float64    `json:"end_lat"`
	EndLng      *float64    `json:"end_lng"`
	StartName   string      `json:"start_name"`
	LinkURL     string      `json:"link_url"`
	Favourite   bool        `json:"favourite"`
	Archived    bool        `json:"archived"`
	Source      string      `json:"source"`
	SourceRef   string      `json:"source_ref"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`

	// Worked out for lists and pages.
	Minutes   int         `json:"minutes"`           // the time it takes (yours, or worked out)
	Level     string      `json:"level"`             // the difficulty in effect
	Done      int         `json:"done"`              // outings done on it
	LastDone  string      `json:"last_done"`         // the last one's date
	Next      string      `json:"next,omitempty"`    // the next planned outing's date
	Points    int         `json:"points"`            // how many track points
	Preview   [][]float64 `json:"preview,omitempty"` // a thin line for lists and the map
	Waypoints []*Waypoint `json:"waypoints,omitempty"`
}

// Waypoint is a spot along a route.
type Waypoint struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"-"`
	RouteID   int64    `json:"route_id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Note      string   `json:"note"`
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
	Ele       *float64 `json:"ele"`
	AtM       float64  `json:"at_m"`
	PlaceRef  string   `json:"place_ref"`
	CreatedAt string   `json:"created_at"`
}

// Gear is one thing you take.
type Gear struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"-"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	WeightG      int    `json:"weight_g"`
	Worn         bool   `json:"worn"`
	Consumable   bool   `json:"consumable"`
	Note         string `json:"note"`
	InventoryRef string `json:"inventory_ref"`
	Archived     bool   `json:"archived"`
	CreatedAt    string `json:"created_at"`

	Packs int `json:"packs"` // on how many packing lists
}

// Pack is a packing list to start from.
type Pack struct {
	ID        int64       `json:"id"`
	UserID    int64       `json:"-"`
	Name      string      `json:"name"`
	Activity  string      `json:"activity"`
	Note      string      `json:"note"`
	CreatedAt string      `json:"created_at"`
	Items     []*PackItem `json:"items"`
	Count     int         `json:"count"`
	Weight    Weights     `json:"weight"`
}

// PackItem is one piece of gear on a packing list.
type PackItem struct {
	GearID int64 `json:"gear_id"`
	Qty    int   `json:"qty"`
	Gear   *Gear `json:"gear,omitempty"`
}

// Weights splits what you carry: the base (the pack without food and
// water), what you wear and what you use up.
type Weights struct {
	Base       int `json:"base"`
	Worn       int `json:"worn"`
	Consumable int `json:"consumable"`
	Total      int `json:"total"` // in the pack: base + consumable
}

// Add counts qty of something weighing g.
func (w *Weights) Add(g, qty int, worn, consumable bool) {
	t := g * max(qty, 1)
	switch {
	case worn:
		w.Worn += t
	case consumable:
		w.Consumable += t
		w.Total += t
	default:
		w.Base += t
		w.Total += t
	}
}

// Person is someone coming along.
type Person struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Ref   string `json:"ref,omitempty"` // Pocket Contacts card
}

// Outing is a route on a day.
type Outing struct {
	ID           int64           `json:"id"`
	UserID       int64           `json:"-"`
	RouteID      *int64          `json:"route_id"`
	Title        string          `json:"title"`
	Activity     string          `json:"activity"`
	Date         string          `json:"date"`
	StartTime    string          `json:"start_time"`
	Status       string          `json:"status"`
	People       []Person        `json:"people"`
	Notes        string          `json:"notes"`
	BackBy       string          `json:"back_by"`
	Safety       bool            `json:"safety"`
	ContactName  string          `json:"contact_name"`
	ContactEmail string          `json:"contact_email"`
	ContactRef   string          `json:"contact_ref"`
	AlertAfter   int             `json:"alert_after"`
	PlanSentAt   string          `json:"plan_sent_at"`
	WarnedAt     string          `json:"warned_at"`
	AlertedAt    string          `json:"alerted_at"`
	AlertError   string          `json:"alert_error"`
	StartedAt    string          `json:"started_at"`
	EndedAt      string          `json:"ended_at"`
	Weather      json.RawMessage `json:"weather"`
	TripRef      string          `json:"trip_ref"`
	TripName     string          `json:"trip_name"`
	CalRef       string          `json:"cal_ref"`
	Rating       int             `json:"rating"`
	Conditions   string          `json:"conditions"`
	Report       string          `json:"report"`
	ActualTrack  []Pt            `json:"actual_track,omitempty"`
	Actual       json.RawMessage `json:"actual"`
	ActualSource string          `json:"actual_source"`
	HealthRef    string          `json:"health_ref"`
	NoteRef      string          `json:"note_ref"`
	Source       string          `json:"source"`
	SourceRef    string          `json:"source_ref"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`

	// Filled for lists and pages.
	RouteName   string        `json:"route_name,omitempty"`
	StartLat    *float64      `json:"start_lat,omitempty"` // where the route starts (Pocket Weather's plans)
	StartLng    *float64      `json:"start_lng,omitempty"`
	DistanceM   float64       `json:"distance_m"` // the route's (or what you did)
	GainM       float64       `json:"gain_m"`
	Minutes     int           `json:"minutes"` // the route's time
	Items       []*OutingItem `json:"items,omitempty"`
	ItemCount   int           `json:"item_count"`
	PackedCount int           `json:"packed_count"`
	Route       *Route        `json:"route,omitempty"`
}

// OutingItem is one thing to pack for an outing.
type OutingItem struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	OutingID   int64  `json:"outing_id"`
	GearID     *int64 `json:"gear_id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	WeightG    int    `json:"weight_g"`
	Qty        int    `json:"qty"`
	Worn       bool   `json:"worn"`
	Consumable bool   `json:"consumable"`
	Packed     bool   `json:"packed"`
	ToBuy      bool   `json:"to_buy"`
	TaskRef    string `json:"task_ref"`
	Position   int    `json:"position"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Routes and the map.
	Units           string `json:"units"`            // metric | imperial
	PacePct         int    `json:"pace_pct"`         // your speed against the usual times, in percent
	DefaultActivity string `json:"default_activity"` // new routes and outings
	MapLayer        string `json:"map_layer"`        // topo | osm
	TrailOverlay    bool   `json:"trail_overlay"`    // marked hiking and cycling routes over the map
	Online          bool   `json:"online"`           // place search, heights, paths and weather from free services

	// Safety: who to tell when you are not back.
	SafetyDefault bool   `json:"safety_default"` // new outings ask someone to be told
	ContactName   string `json:"contact_name"`
	ContactEmail  string `json:"contact_email"`
	ContactRef    string `json:"contact_ref"`
	AlertAfter    int    `json:"alert_after"` // minutes after the back-by time

	// The other apps.
	CalendarSync bool   `json:"calendar_sync"` // outings go to Pocket Calendar
	HealthLog    bool   `json:"health_log"`    // a finished outing goes to Pocket Health as a workout
	MovementAuto bool   `json:"movement_auto"` // a finished outing takes its track from Pocket Movement
	MailFrom     string `json:"mail_from"`     // which Pocket Mail account emails leave from ("" = its default)
	AIHelp       bool   `json:"ai_help"`

	// Notifications.
	NotifyEvening bool   `json:"notify_evening"` // the evening before: weather, sunset, packing
	EveningHour   int    `json:"evening_hour"`
	NotifyMorning bool   `json:"notify_morning"` // the morning of an outing
	NotifyHour    int    `json:"notify_hour"`
	NotifyURL     string `json:"notify_url"`
	NotifyFormat  string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// CleanTags trims, de-duplicates and caps a list of tags.
func CleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
