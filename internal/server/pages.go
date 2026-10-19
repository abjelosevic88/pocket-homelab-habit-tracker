package server

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

var monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func (s *Server) handleMonth(w http.ResponseWriter, r *http.Request) {
	month := chi.URLParam(r, "month")
	if month == "current" {
		month = store.Today()[:7]
	}
	if !monthRe.MatchString(month) {
		writeErr(w, http.StatusBadRequest, "month must be YYYY-MM")
		return
	}
	page, err := s.store.Month(r.Context(), me(r), month)
	if err != nil {
		s.fail(w, err, "load month")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	page, err := s.store.Day(r.Context(), me(r), date)
	if err != nil {
		s.fail(w, err, "load day")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	page, err := s.store.Day(r.Context(), me(r), store.Today())
	if err != nil {
		s.fail(w, err, "load today")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleCheckin writes a whole day in one call — meant for iOS Shortcuts,
// Home Assistant and similar automations.
//
//	{"date":"2026-10-11","habits":{"1":1,"2":-1},"trackers":{"1":4,"3":81.2},"journal":"Great walk.","important":"Moved in."}
func (s *Server) handleCheckin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Date      string              `json:"date"`
		Habits    map[string]int      `json:"habits"`
		Trackers  map[string]*float64 `json:"trackers"`
		Journal   *string             `json:"journal"`
		Bad       *string             `json:"journal_bad"`
		Important *string             `json:"important"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if in.Date > store.Today() {
		writeErr(w, http.StatusBadRequest, "cannot check in a future day")
		return
	}
	ctx := r.Context()
	uid := me(r)
	for k, st := range in.Habits {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil || (st != 1 && st != -1 && st != 0) {
			writeErr(w, http.StatusBadRequest, "habits must map id -> 1 | -1 | 0")
			return
		}
		if err := s.store.SetHabitEntry(ctx, uid, id, in.Date, st); err != nil {
			s.fail(w, err, "set habit")
			return
		}
	}
	for k, v := range in.Trackers {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "trackers must map id -> number | null")
			return
		}
		if err := s.store.SetTrackerEntry(ctx, uid, id, in.Date, v); err != nil {
			s.fail(w, err, "set tracker")
			return
		}
	}
	if in.Journal != nil || in.Bad != nil || in.Important != nil {
		if err := s.store.SetJournalFields(ctx, uid, in.Date, in.Journal, in.Bad, in.Important); err != nil {
			s.fail(w, err, "set journal")
			return
		}
	}
	page, err := s.store.Day(ctx, uid, in.Date)
	if err != nil {
		s.fail(w, err, "load day")
		return
	}
	s.publish(userFrom(r.Context()), "habits.checkin", "day:"+in.Date, map[string]any{
		"date": in.Date, "habits": len(in.Habits), "trackers": len(in.Trackers), "journal": in.Journal != nil || in.Bad != nil || in.Important != nil,
	})
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.DashboardStats(r.Context(), me(r))
	if err != nil {
		s.fail(w, err, "dashboard")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleInsights accepts ?from=&to= or ?months=N (default 3, ending today).
func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		months := 3
		if n, err := strconv.Atoi(q.Get("months")); err == nil && n > 0 && n <= 60 {
			months = n
		}
		now := time.Now()
		to = now.Format("2006-01-02")
		from = time.Date(now.Year(), now.Month()-time.Month(months-1), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	}
	if !validDate(from) || !validDate(to) || from > to {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD and from <= to")
		return
	}
	ins, err := s.store.InsightsRange(r.Context(), me(r), from, to)
	if err != nil {
		s.fail(w, err, "insights")
		return
	}
	writeJSON(w, http.StatusOK, ins)
}

func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if to == "" {
		to = store.Today()
	}
	if from == "" {
		from = "1970-01-01"
	}
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
		return
	}
	j, err := s.store.Journal(r.Context(), me(r), from, to)
	if err != nil {
		s.fail(w, err, "journal")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleSetJournal(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	var in struct {
		Text      *string `json:"text"`
		Bad       *string `json:"bad"`
		Important *string `json:"important"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// An important note may go on a day ahead (an appointment, a trip); the journal lines may not.
	if date > store.Today() && (in.Text != nil || in.Bad != nil) {
		writeErr(w, http.StatusBadRequest, "cannot write a future day")
		return
	}
	for _, v := range []*string{in.Text, in.Bad, in.Important} {
		if v != nil && len(*v) > 4000 {
			writeErr(w, http.StatusBadRequest, "keep it under 4000 characters")
			return
		}
	}
	ctx := r.Context()
	uid := me(r)
	if in.Text != nil || in.Bad != nil || in.Important != nil {
		if err := s.store.SetJournalFields(ctx, uid, date, in.Text, in.Bad, in.Important); err != nil {
			s.fail(w, err, "set journal")
			return
		}
	}
	full, _ := s.store.JournalFull(ctx, uid, date, date)
	writeJSON(w, http.StatusOK, map[string]string{"date": date, "text": full[date].Text, "bad": full[date].Bad, "important": full[date].Important})
}
