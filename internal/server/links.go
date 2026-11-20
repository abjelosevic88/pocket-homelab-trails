package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-trails/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Trails as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "trails:read", Name: "See your routes and outings", Description: "Read your routes (distance, climb, time, the spots along them, the GPX), your gear and packing lists, your planned and finished outings with their weather, who comes and how it went, and your year's totals — for your journal, your calendar's weather, a trip's page, search and Pocket AI."},
	{Scope: "trails:write", Name: "Plan outings and add routes", Description: "Plan an outing on one of your routes or add a route from a track (\"we hike Trebević on Saturday\", a ride you recorded). It may change or remove only the outings and routes it added itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "trails.json", Kind: core.KindPocket, Title: "Every route with its track and waypoints, your outings with what you walked, your gear and packing lists"},
		{Path: "/api/v1/export.csv", File: "outings.csv", Kind: core.KindCSV, Title: "Every outing as a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", Title: "Adds the routes, outings, gear and packing lists from another box; what is already here is skipped"},
		{Path: "/api/v1/routes/import", Kind: core.KindGPX, Form: "file", Title: "Adds each track or route in a GPX file as a route, with its waypoints; the same file twice adds nothing"},
		{Path: "/api/v1/routes/import", Kind: "kml", Form: "file", Title: "Adds each line in a KML file (Google My Maps, Google Earth) as a route; the same file twice adds nothing"},
		{Path: "/api/v1/routes/import", Kind: "tcx", Form: "file", Title: "Adds a Garmin TCX course or activity as a route; the same file twice adds nothing"},
		{Path: "/api/v1/gear/import", Kind: "lighterpack-csv", Form: "file", Title: "Adds the gear in a LighterPack (or any Name, Weight) CSV and makes a packing list named after the file; gear and lists already here are kept"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(summary|today|stats|routes|outings|gear|packs|search|day/\d{4}-\d{2}-\d{2})$`), "trails:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(routes|outings)/\d+$`), "trails:read"},
	{"GET", regexp.MustCompile(`^/api/v1/routes/\d+/gpx$`), "trails:read"},
	{"POST", regexp.MustCompile(`^/api/v1/(outings|routes)$`), "trails:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/(outings|routes)/\d+$`), "trails:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "trails" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
