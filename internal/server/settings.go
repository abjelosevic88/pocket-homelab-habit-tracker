package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.ReminderTime != "" && !timeRe.MatchString(in.ReminderTime) {
		writeErr(w, http.StatusBadRequest, "reminder_time must be HH:MM")
		return
	}
	if in.WeekStart < 1 || in.WeekStart > 7 {
		in.WeekStart = 1
	}
	if in.Landing != "today" {
		in.Landing = "month"
	}
	switch in.ReminderFormat {
	case "ntfy", "apprise", "json":
	default:
		in.ReminderFormat = "ntfy"
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = "Pocket Habits"
	}
	if strings.TrimSpace(in.JournalPrompt) == "" {
		in.JournalPrompt = "What was great about today?"
	}
	if strings.TrimSpace(in.BadPrompt) == "" {
		in.BadPrompt = "And one thing that was bad?"
	}
	if in.WeatherUnits != "fahrenheit" {
		in.WeatherUnits = "celsius"
	}
	if in.WeatherLat < -90 || in.WeatherLat > 90 || in.WeatherLon < -180 || in.WeatherLon > 180 {
		writeErr(w, http.StatusBadRequest, "weather_lat/weather_lon out of range")
		return
	}
	if in.WeatherEnabled && in.WeatherLat == 0 && in.WeatherLon == 0 {
		writeErr(w, http.StatusBadRequest, "set the home location before turning the weather on")
		return
	}
	in.WeatherPlace = strings.TrimSpace(in.WeatherPlace)
	in.ReminderLast, in.SetupComplete, in.DefaultsSeeded = cur.ReminderLast, cur.SetupComplete, cur.DefaultsSeeded
	// A changed place starts the fetches over: the stored days were for the old one.
	in.WeatherLast = cur.WeatherLast
	moved := in.WeatherLat != cur.WeatherLat || in.WeatherLon != cur.WeatherLon
	if moved || (in.WeatherEnabled && !cur.WeatherEnabled) {
		in.WeatherLast = ""
	}
	if err := s.store.SaveSettings(r.Context(), in); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if in.WeatherEnabled && (moved || !cur.WeatherEnabled) {
		// Fetch the recent days right away so the pages show something; the rest trickles in.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			from := time.Now().AddDate(0, 0, -45).Format("2006-01-02")
			if _, err := s.refreshWeather(ctx, in, from, store.Today()); err != nil {
				s.log.Warn("weather first fetch failed", "err", err)
			}
		}()
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"scheduler": s.cfg.SchedulerEnabled, "counts": s.store.Counts(r.Context()),
	})
}

// coreStatus summarises the Pocket Core link for the About page.
func (s *Server) coreStatus() map[string]any {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return map[string]any{"configured": s.core.Configured(), "url": s.cfg.CoreURL, "registered": s.coreOK, "error": s.coreErr}
}

// ---------- reminders ----------

// RunScheduler is called every minute: it sends the nightly reminder once per
// day at the configured time when something is still unmarked.
func (s *Server) RunScheduler(ctx context.Context) {
	s.RunWeather(ctx)
	st, err := s.store.GetSettings(ctx)
	if err != nil || st.ReminderTime == "" || st.ReminderURL == "" {
		return
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	if st.ReminderLast == today || now.Format("15:04") < st.ReminderTime {
		return
	}
	day, err := s.store.Day(ctx, today)
	if err != nil {
		return
	}
	left := day.Total - day.Done
	unmarked := 0
	for _, h := range day.Habits {
		if h.Scheduled && h.Status == 0 {
			unmarked++
		}
	}
	// mark as sent first so a failing endpoint does not retry every minute
	st.ReminderLast = today
	_ = s.store.SetSetting(ctx, "reminder_last", today)
	if unmarked == 0 && day.Journal != "" {
		return
	}
	msg := fmt.Sprintf("%d of %d habits still open today", left, day.Total)
	if unmarked == 0 {
		msg = "Habits are marked — one line for the journal?"
	}
	if err := s.sendReminder(ctx, st, "Evening check-in", msg); err != nil {
		s.log.Warn("reminder failed", "err", err)
	}
}

func (s *Server) sendReminder(ctx context.Context, st store.Settings, title, msg string) error {
	var body []byte
	ct := "text/plain; charset=utf-8"
	switch st.ReminderFormat {
	case "apprise":
		body, _ = json.Marshal(map[string]string{"title": title, "body": msg, "type": "info"})
		ct = "application/json"
	case "json":
		body, _ = json.Marshal(map[string]string{"title": title, "message": msg, "url": s.cfg.BaseURL + "/today"})
		ct = "application/json"
	default:
		body = []byte(msg)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, st.ReminderURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ct)
	if st.ReminderFormat == "ntfy" {
		req.Header.Set("Title", title)
		req.Header.Set("Tags", "white_check_mark")
		if s.cfg.BaseURL != "" {
			req.Header.Set("Click", s.cfg.BaseURL+"/today")
		}
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("reminder endpoint returned %s", res.Status)
	}
	return nil
}

func (s *Server) handleTestReminder(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.ReminderURL == "" {
		writeErr(w, http.StatusBadRequest, "reminder_url is required")
		return
	}
	if err := s.sendReminder(r.Context(), in, "Pocket Habits", "Test notification — reminders are wired up."); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- backup / export / import ----------

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-habits-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// Export is the portable JSON shape (also accepted by /import).
type Export struct {
	Version        string                 `json:"version"`
	ExportedAt     string                 `json:"exported_at"`
	Settings       store.Settings         `json:"settings"`
	Habits         []store.Habit          `json:"habits"`
	HabitEntries   []habitEntryRow        `json:"habit_entries"`
	Trackers       []store.Tracker        `json:"trackers"`
	TrackerEntries []trackerEntryRow      `json:"tracker_entries"`
	Journal        []store.JournalLine    `json:"journal"`
	Meta           map[string]interface{} `json:"meta,omitempty"`
}

type habitEntryRow struct {
	HabitID int64  `json:"habit_id"`
	Date    string `json:"date"`
	Status  int    `json:"status"`
}

type trackerEntryRow struct {
	TrackerID int64   `json:"tracker_id"`
	Date      string  `json:"date"`
	Value     float64 `json:"value"`
}

func (s *Server) buildExport(ctx context.Context) (*Export, error) {
	st, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	habits, err := s.store.ListHabits(ctx, "", true)
	if err != nil {
		return nil, err
	}
	trackers, err := s.store.ListTrackers(ctx, "", true)
	if err != nil {
		return nil, err
	}
	he, err := s.store.HabitEntries(ctx, "0000-01-01", "9999-12-31")
	if err != nil {
		return nil, err
	}
	te, err := s.store.TrackerEntries(ctx, "0000-01-01", "9999-12-31")
	if err != nil {
		return nil, err
	}
	j, err := s.store.JournalFull(ctx, "0000-01-01", "9999-12-31")
	if err != nil {
		return nil, err
	}
	ex := &Export{Version: Version, ExportedAt: store.Now(), Settings: st, Habits: habits, Trackers: trackers,
		HabitEntries: []habitEntryRow{}, TrackerEntries: []trackerEntryRow{}, Journal: []store.JournalLine{}}
	for _, h := range habits {
		for d, st := range he[h.ID] {
			ex.HabitEntries = append(ex.HabitEntries, habitEntryRow{h.ID, d, st})
		}
	}
	for _, t := range trackers {
		for d, v := range te[t.ID] {
			ex.TrackerEntries = append(ex.TrackerEntries, trackerEntryRow{t.ID, d, v})
		}
	}
	for d, e := range j {
		ex.Journal = append(ex.Journal, store.JournalLine{Date: d, Text: e.Text, Bad: e.Bad, Important: e.Important})
	}
	sortRows(ex)
	return ex, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ex, err := s.buildExport(r.Context())
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-habits-export.json"`)
	writeJSON(w, http.StatusOK, ex)
}

// handleExportCSV writes one long-format table: date,type,id,name,value.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ex, err := s.buildExport(r.Context())
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	names := map[string]string{}
	for _, h := range ex.Habits {
		names["habit:"+strconv.FormatInt(h.ID, 10)] = h.Name
	}
	for _, t := range ex.Trackers {
		names["tracker:"+strconv.FormatInt(t.ID, 10)] = t.Name
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-habits-export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "type", "id", "name", "value"})
	for _, e := range ex.HabitEntries {
		v := "done"
		if e.Status == -1 {
			v = "missed"
		}
		_ = cw.Write([]string{e.Date, "habit", strconv.FormatInt(e.HabitID, 10), names["habit:"+strconv.FormatInt(e.HabitID, 10)], v})
	}
	for _, e := range ex.TrackerEntries {
		_ = cw.Write([]string{e.Date, "tracker", strconv.FormatInt(e.TrackerID, 10), names["tracker:"+strconv.FormatInt(e.TrackerID, 10)], strconv.FormatFloat(e.Value, 'f', -1, 64)})
	}
	for _, j := range ex.Journal {
		if j.Text != "" {
			_ = cw.Write([]string{j.Date, "journal", "", "", j.Text})
		}
		if j.Bad != "" {
			_ = cw.Write([]string{j.Date, "journal_bad", "", "", j.Bad})
		}
		if j.Important != "" {
			_ = cw.Write([]string{j.Date, "important", "", "", j.Important})
		}
	}
	cw.Flush()
}

// handleImportJSON merges an export file: habits/trackers are matched by name
// (created when missing), entries and journal lines are upserted.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	var ex Export
	if err := decode(r, &ex); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := r.Context()
	existingH, _ := s.store.ListHabits(ctx, "", true)
	existingT, _ := s.store.ListTrackers(ctx, "", true)
	hmap := map[int64]int64{}
	tmap := map[int64]int64{}
	created := 0
	for _, h := range ex.Habits {
		var id int64
		for _, e := range existingH {
			if strings.EqualFold(e.Name, h.Name) {
				id = e.ID
				break
			}
		}
		if id == 0 {
			nh := h
			nh.ID = 0
			if err := s.store.CreateHabit(ctx, &nh); err != nil {
				s.fail(w, err, "import habit")
				return
			}
			id = nh.ID
			created++
		}
		hmap[h.ID] = id
	}
	for _, t := range ex.Trackers {
		var id int64
		for _, e := range existingT {
			if strings.EqualFold(e.Name, t.Name) {
				id = e.ID
				break
			}
		}
		if id == 0 {
			nt := t
			nt.ID = 0
			if err := s.store.CreateTracker(ctx, &nt); err != nil {
				s.fail(w, err, "import tracker")
				return
			}
			id = nt.ID
			created++
		}
		tmap[t.ID] = id
	}
	n := 0
	for _, e := range ex.HabitEntries {
		if id, ok := hmap[e.HabitID]; ok && validDate(e.Date) {
			if err := s.store.SetHabitEntry(ctx, id, e.Date, e.Status); err == nil {
				n++
			}
		}
	}
	for _, e := range ex.TrackerEntries {
		if id, ok := tmap[e.TrackerID]; ok && validDate(e.Date) {
			v := e.Value
			if err := s.store.SetTrackerEntry(ctx, id, e.Date, &v); err == nil {
				n++
			}
		}
	}
	for _, j := range ex.Journal {
		if validDate(j.Date) && (j.Text != "" || j.Bad != "" || j.Important != "") {
			text, bad, important := j.Text, j.Bad, j.Important
			if err := s.store.SetJournalFields(ctx, j.Date, &text, &bad, &important); err == nil {
				n++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "entries": n})
}
