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
