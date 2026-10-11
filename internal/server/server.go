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
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

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
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	return &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now()}
}

// Bootstrap seeds the first admin (if configured), default trackers and demo data.
func (s *Server) Bootstrap(ctx context.Context) error {
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
		st, _ := s.store.GetSettings(ctx)
		st.SetupComplete = true
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	st, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !st.DefaultsSeeded {
		// The notebook's three bottom rows: sleep, day rating, weight.
		defaults := []store.Tracker{
			{Name: "Sleep score", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true},
			{Name: "Day rating", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true},
			{Name: "Weight", Kind: "number", MinValue: 0, MaxValue: 0, Step: 0.1, Unit: "kg", HigherIsBetter: false},
		}
		first := time.Now().AddDate(0, 0, -time.Now().Day()+1).Format("2006-01-02")
		for i := range defaults {
			defaults[i].StartDate = first
			if err := s.store.CreateTracker(ctx, &defaults[i]); err != nil {
				return err
			}
		}
		st.DefaultsSeeded = true
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
	}
	if s.cfg.DemoData {
		if err := s.seedDemo(ctx); err != nil {
			return err
		}
	}
	_ = s.store.PurgeSessions(ctx)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
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
	counts := s.store.Counts(ctx)
	d, _ := s.store.DashboardStats(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_habits_info Build info.\n# TYPE pocket_habits_info gauge\npocket_habits_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_habits_uptime_seconds Process uptime.\n# TYPE pocket_habits_uptime_seconds gauge\npocket_habits_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_habits_http_requests_total HTTP requests served.\n# TYPE pocket_habits_http_requests_total counter\npocket_habits_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_habits_http_errors_total HTTP 5xx responses.\n# TYPE pocket_habits_http_errors_total counter\npocket_habits_http_errors_total %d\n", s.errors.Load())
	for k, v := range counts {
		fmt.Fprintf(w, "# TYPE pocket_habits_rows{table=%q} gauge\npocket_habits_rows{table=%q} %d\n", k, k, v)
	}
	if d != nil {
		fmt.Fprintf(w, "# HELP pocket_habits_today_done Habits done today.\n# TYPE pocket_habits_today_done gauge\npocket_habits_today_done %d\n", d.TodayDone)
		fmt.Fprintf(w, "# HELP pocket_habits_today_total Habits scheduled today.\n# TYPE pocket_habits_today_total gauge\npocket_habits_today_total %d\n", d.TodayTotal)
		fmt.Fprintf(w, "# HELP pocket_habits_month_done Habits done this month.\n# TYPE pocket_habits_month_done gauge\npocket_habits_month_done %d\n", d.MonthDone)
		fmt.Fprintf(w, "# HELP pocket_habits_month_missed Habits missed this month.\n# TYPE pocket_habits_month_missed gauge\npocket_habits_month_missed %d\n", d.MonthMissed)
		fmt.Fprintf(w, "# HELP pocket_habits_perfect_streak Consecutive all-green days.\n# TYPE pocket_habits_perfect_streak gauge\npocket_habits_perfect_streak %d\n", d.PerfectStreak)
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
