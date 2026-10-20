package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core): another
// Pocket app may act here for a person with a pcx_ token, only within these
// scopes. Every route not listed in linkRoutes stays closed to it.

var linkOffers = []core.Offer{
	{Scope: "habits:read", Name: "See your habits", Description: "List your habits and see a day's check-ins, so the app can show or tick them."},
	{Scope: "entries:write", Name: "Tick off habits", Description: "Check a habit in or out for a day, for example \"Cooked at home\" when you cook a recipe."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/habits$`), "habits:read"},
	{"GET", regexp.MustCompile(`^/api/v1/habits/\d+$`), "habits:read"},
	{"GET", regexp.MustCompile(`^/api/v1/days/\d{4}-\d{2}-\d{2}$`), "habits:read"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/habits/\d+/entries/\d{4}-\d{2}-\d{2}$`), "entries:write"},
}

var errLinkScope = errors.New("outside the link's scope")

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

// linkUser validates a pcx_ token and maps it to the person it acts for; the
// second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	app := "link"
	if in.App != nil && in.App.Slug != "" {
		app = in.App.Slug
	}
	u, err := s.firstOrSystemUser(ctx, strings.ToLower(in.User.Email), in.User.Name)
	return u, app, err
}

// linkApp is the slug of the Pocket app acting through a link token, "" otherwise.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

// viaLink names the acting app in an event payload ("via": "kitchen") when a
// linked app made the change.
func viaLink(r *http.Request, payload map[string]any) map[string]any {
	if app := linkApp(r.Context()); app != "" {
		payload["via"] = app
	}
	return payload
}
