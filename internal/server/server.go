// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/config"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	webFS    fs.FS
	started  time.Time
	requests atomic.Int64
	errors   atomic.Int64

	core     *core.Client
	coreMu   sync.Mutex
	coreOK   bool
	coreErr  string
	resolved map[string]resolvedURL

	oidc   *oidcProvider // keyed on the issuer; rebuilt by provider() when it changes
	oidcMu sync.Mutex
}

type resolvedURL struct {
	url string
	at  time.Time
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	return &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(),
		core:     core.New(cfg.CoreURL, core.Options{DB: st.DB, TokenFile: cfg.DataDir + "/core.token"}),
		resolved: map[string]resolvedURL{},
		oidc:     &oidcProvider{issuerURL: cfg.OIDCIssuer}}
}

// Bootstrap seeds the first admin (if configured), default trackers and demo data.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := s.core.Start(ctx); err != nil {
		return err
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		if err := s.store.SetBoxSetting(ctx, "setup_complete", "1"); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	// Every account gets the notebook's default trackers (and the demo data when asked for)
	// once, in its own dataset.
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	for i := range users {
		if err := s.setupUser(ctx, &users[i]); err != nil {
			return err
		}
	}
	_ = s.store.PurgeSessions(ctx)
	if s.core.Configured() {
		go s.coreLoop(ctx)
	}
	return nil
}

// setupUser seeds a user's own dataset once: the notebook's three bottom rows (sleep, day
// rating, weight) and, with DEMO_DATA, the demo history.
func (s *Server) setupUser(ctx context.Context, u *store.User) error {
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		return err
	}
	if !st.DefaultsSeeded {
		if err := s.seedDefaultTrackers(ctx, u.ID); err != nil {
			return err
		}
		if err := s.store.SetSetting(ctx, u.ID, "defaults_seeded", "1"); err != nil {
			return err
		}
	}
	if s.cfg.DemoData {
		return s.seedDemo(ctx, u.ID) // a no-op once the user has habits
	}
	return nil
}

func (s *Server) seedDefaultTrackers(ctx context.Context, uid int64) error {
	defaults := []store.Tracker{
		{Name: "Sleep score", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true},
		{Name: "Day rating", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true},
		{Name: "Weight", Kind: "number", MinValue: 0, MaxValue: 0, Step: 0.1, Unit: "kg", HigherIsBetter: false},
	}
	first := time.Now().AddDate(0, 0, -time.Now().Day()+1).Format("2006-01-02")
	for i := range defaults {
		defaults[i].StartDate = first
		if err := s.store.CreateTracker(ctx, uid, &defaults[i]); err != nil {
			return err
		}
	}
	return nil
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/pocket-ask.js", s.core.AskWidget("habits"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Get("/auth/oidc/start", s.handleOIDCStart)
		r.Get("/auth/oidc/callback", s.handleOIDCCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/months/{month}", s.handleMonth)
			r.Get("/days/{date}", s.handleDay)
			r.Get("/today", s.handleToday)
			r.Post("/checkin", s.handleCheckin)
			r.Get("/dashboard", s.handleDashboard)
			r.Get("/insights", s.handleInsights)

			r.Get("/habits", s.handleListHabits)
			r.Post("/habits", s.handleCreateHabit)
			r.Put("/habits/reorder", s.handleReorderHabits)
			r.Get("/habits/{id}", s.handleGetHabit)
			r.Put("/habits/{id}", s.handleUpdateHabit)
			r.Delete("/habits/{id}", s.handleDeleteHabit)
			r.Post("/habits/{id}/end", s.handleEndHabit)
			r.Post("/habits/{id}/resume", s.handleResumeHabit)
			r.Put("/habits/{id}/entries/{date}", s.handleSetHabitEntry)
			r.Delete("/habits/{id}/entries/{date}", s.handleClearHabitEntry)

			r.Get("/trackers", s.handleListTrackers)
			r.Post("/trackers", s.handleCreateTracker)
			r.Put("/trackers/reorder", s.handleReorderTrackers)
			r.Get("/trackers/{id}", s.handleGetTracker)
			r.Put("/trackers/{id}", s.handleUpdateTracker)
			r.Delete("/trackers/{id}", s.handleDeleteTracker)
			r.Post("/trackers/{id}/end", s.handleEndTracker)
			r.Post("/trackers/{id}/resume", s.handleResumeTracker)
			r.Put("/trackers/{id}/entries/{date}", s.handleSetTrackerEntry)
			r.Delete("/trackers/{id}/entries/{date}", s.handleClearTrackerEntry)

			r.Get("/journal", s.handleJournal)
			r.Put("/journal/{date}", s.handleSetJournal)

			r.Get("/weather", s.handleWeather)
			r.Put("/weather/{date}", s.handleSetWeather)
			r.Post("/weather/refresh", s.handleRefreshWeather)
			r.Get("/weather/geocode", s.handleGeocode)
			r.Get("/location", s.handleLocations)
			r.Post("/location", s.handleSetLocation)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Post("/settings/test-reminder", s.handleTestReminder)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
			r.Get("/export.json", s.handleExportJSON)
			r.Get("/export.csv", s.handleExportCSV)
			r.Post("/import", s.handleImportJSON)
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	return dec.Decode(v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func validDate(s string) bool {
	_, err := store.ParseDate(s)
	return err == nil
}

// ---------- health / metrics ----------

// ---- Pocket Core ----

// coreLoop registers with Core on start (retrying until it answers) and
// re-registers every 30 minutes so version and last-seen stay fresh.
func (s *Server) coreLoop(ctx context.Context) {
	wait := 5 * time.Second
	for {
		err := s.registerWithCore(ctx)
		s.coreMu.Lock()
		s.coreOK, s.coreErr = err == nil, ""
		if err != nil {
			s.coreErr = err.Error()
		}
		s.coreMu.Unlock()
		if err != nil {
			s.log.Warn("pocket core: registration failed, retrying", "err", err, "in", wait.String())
		} else {
			wait = 30 * time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err != nil && wait < 5*time.Minute {
			wait *= 2
		}
	}
}

func (s *Server) registerWithCore(ctx context.Context) error {
	m := core.Manifest{
		Slug: "habits", Name: "Pocket Habits",
		Description: "A self-hosted habit tracker that copies the one-page bullet-journal spread that actually sticks.",
		Icon:        "/favicon.svg", Version: Version,
		URL: s.cfg.CoreAppURL, PublicURL: s.cfg.BaseURL, HealthPath: "/healthz", EnvPrefix: "HABITS",
		Publishes: []string{"habits.marked", "habits.unmarked", "habits.tracked", "habits.checkin"},
		Offers:    linkOffers,
	}
	if s.cfg.BaseURL != "" {
		// ask for a relying-party client so "Sign in with Pocket Core" needs no configuration
		m.OIDC = &core.OIDCRequest{RedirectURIs: []string{s.cfg.BaseURL + "/api/v1/auth/oidc/callback"}}
	}
	reg, err := s.core.Register(ctx, s.cfg.CoreRegistrationKey, m)
	if err != nil {
		return err
	}
	if reg.Token != "" {
		s.log.Info("pocket core: registered, app token stored", "core", s.cfg.CoreURL)
	} else {
		s.log.Debug("pocket core: re-registered", "core", s.cfg.CoreURL)
	}
	return nil
}

// publish persists an event locally; the SDK retries network delivery. Every event names the person
// it is about (the account's email, which is how Core and the sibling apps know people), so a
// consumer files it under that person and nobody else.

// resolveSibling asks Core where a sibling app lives (cached a minute) so the
// *_URL settings may stay empty on a box with Core.
func (s *Server) resolveSibling(ctx context.Context, slug string) string {
	if !s.core.Configured() {
		return ""
	}
	s.coreMu.Lock()
	if r, ok := s.resolved[slug]; ok && time.Since(r.at) < time.Minute {
		s.coreMu.Unlock()
		return r.url
	}
	s.coreMu.Unlock()
	url := ""
	if res, err := s.core.Resolve(ctx, slug); err == nil {
		url = res.URL
	}
	s.coreMu.Lock()
	s.resolved[slug] = resolvedURL{url: url, at: time.Now()}
	s.coreMu.Unlock()
	return url
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Box-wide row counts only: the metrics endpoint is unauthenticated and nobody's
	// habits are anybody else's business.
	counts := s.store.Counts(ctx, 0)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_habits_info Build info.\n# TYPE pocket_habits_info gauge\npocket_habits_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_habits_uptime_seconds Process uptime.\n# TYPE pocket_habits_uptime_seconds gauge\npocket_habits_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_habits_http_requests_total HTTP requests served.\n# TYPE pocket_habits_http_requests_total counter\npocket_habits_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_habits_http_errors_total HTTP 5xx responses.\n# TYPE pocket_habits_http_errors_total counter\npocket_habits_http_errors_total %d\n", s.errors.Load())
	for k, v := range counts {
		fmt.Fprintf(w, "# TYPE pocket_habits_rows{table=%q} gauge\npocket_habits_rows{table=%q} %d\n", k, k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
