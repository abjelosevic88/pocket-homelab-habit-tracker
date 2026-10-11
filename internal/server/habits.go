package server

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

var daysRe = regexp.MustCompile(`^[1-7]{1,7}$`)

func cleanDays(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return "1234567"
	}
	seen := map[rune]bool{}
	var out []rune
	for _, r := range "1234567" {
		if strings.ContainsRune(d, r) && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	if len(out) == 0 || !daysRe.MatchString(string(out)) {
		return "1234567"
	}
	return string(out)
}

// endOfMonth returns the last day of the month containing date.
func endOfMonth(date string) string {
	t, err := store.ParseDate(date)
	if err != nil {
		t = time.Now()
	}
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// ---- habits ----

func (s *Server) handleListHabits(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	h, err := s.store.ListHabits(r.Context(), r.URL.Query().Get("month"), all)
	if err != nil {
		s.fail(w, err, "list habits")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleGetHabit(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.GetHabit(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get habit")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

type habitInput struct {
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	Days      string  `json:"days"`
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

func (in habitInput) validate() string {
	if strings.TrimSpace(in.Name) == "" {
		return "name is required"
	}
	if in.StartDate != "" && !validDate(in.StartDate) {
		return "start_date must be YYYY-MM-DD"
	}
	if in.EndDate != nil && *in.EndDate != "" && !validDate(*in.EndDate) {
		return "end_date must be YYYY-MM-DD"
	}
	return ""
}

func (s *Server) handleCreateHabit(w http.ResponseWriter, r *http.Request) {
	var in habitInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	h := &store.Habit{Name: strings.TrimSpace(in.Name), Color: in.Color, Days: cleanDays(in.Days), StartDate: in.StartDate}
	if in.EndDate != nil && *in.EndDate != "" {
		h.EndDate = in.EndDate
	}
	if err := s.store.CreateHabit(r.Context(), h); err != nil {
		s.fail(w, err, "create habit")
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleUpdateHabit(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.GetHabit(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get habit")
		return
	}
	var in habitInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	h.Name, h.Color, h.Days = strings.TrimSpace(in.Name), in.Color, cleanDays(in.Days)
	if in.StartDate != "" {
		h.StartDate = in.StartDate
	}
	if in.EndDate == nil || *in.EndDate == "" {
		h.EndDate = nil
	} else {
		h.EndDate = in.EndDate
	}
	if err := s.store.UpdateHabit(r.Context(), h); err != nil {
		s.fail(w, err, "update habit")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleDeleteHabit(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteHabit(r.Context(), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete habit")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEndHabit retires a habit at the end of the given month (default: current) —
// it stays on that month's page and disappears from the next one.
func (s *Server) handleEndHabit(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.GetHabit(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get habit")
		return
	}
	var in struct {
		Date string `json:"date"`
	}
	_ = decode(r, &in)
	end := endOfMonth(in.Date)
	if in.Date != "" && r.URL.Query().Get("exact") == "1" {
		end = in.Date
	}
	if end < h.StartDate {
		end = h.StartDate
	}
	h.EndDate = &end
	if err := s.store.UpdateHabit(r.Context(), h); err != nil {
		s.fail(w, err, "end habit")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleResumeHabit(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.GetHabit(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get habit")
		return
	}
	h.EndDate = nil
	if err := s.store.UpdateHabit(r.Context(), h); err != nil {
		s.fail(w, err, "resume habit")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleReorderHabits(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderHabits(r.Context(), in.IDs); err != nil {
		s.fail(w, err, "reorder habits")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetHabitEntry(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if date > store.Today() {
		writeErr(w, http.StatusBadRequest, "cannot mark a day in the future")
		return
	}
	if _, err := s.store.GetHabit(r.Context(), idParam(r, "id")); err != nil {
		s.fail(w, err, "get habit")
		return
	}
	var in struct {
		Status int `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Status != 1 && in.Status != -1 && in.Status != 0 {
		writeErr(w, http.StatusBadRequest, "status must be 1 (done), -1 (missed) or 0 (clear)")
		return
	}
	if err := s.store.SetHabitEntry(r.Context(), idParam(r, "id"), date, in.Status); err != nil {
		s.fail(w, err, "set entry")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"habit_id": idParam(r, "id"), "date": date, "status": in.Status})
}

func (s *Server) handleClearHabitEntry(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if err := s.store.SetHabitEntry(r.Context(), idParam(r, "id"), date, 0); err != nil {
		s.fail(w, err, "clear entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- trackers ----

func (s *Server) handleListTrackers(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	t, err := s.store.ListTrackers(r.Context(), r.URL.Query().Get("month"), all)
	if err != nil {
		s.fail(w, err, "list trackers")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetTracker(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTracker(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get tracker")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type trackerInput struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	MinValue       *float64 `json:"min_value"`
	MaxValue       *float64 `json:"max_value"`
	Step           *float64 `json:"step"`
	Unit           string   `json:"unit"`
	HigherIsBetter *bool    `json:"higher_is_better"`
	StartDate      string   `json:"start_date"`
	EndDate        *string  `json:"end_date"`
}

func (in trackerInput) apply(t *store.Tracker) string {
	if strings.TrimSpace(in.Name) == "" {
		return "name is required"
	}
	t.Name = strings.TrimSpace(in.Name)
	if in.Kind != "" {
		t.Kind = in.Kind
	}
	if t.Kind != "scale" && t.Kind != "number" {
		return "kind must be scale or number"
	}
	if in.MinValue != nil {
		t.MinValue = *in.MinValue
	}
	if in.MaxValue != nil {
		t.MaxValue = *in.MaxValue
	}
	if in.Step != nil && *in.Step > 0 {
		t.Step = *in.Step
	}
	if t.Step <= 0 {
		t.Step = 1
	}
	t.Unit = strings.TrimSpace(in.Unit)
	if in.HigherIsBetter != nil {
		t.HigherIsBetter = *in.HigherIsBetter
	}
	if t.Kind == "scale" {
		if t.MaxValue <= t.MinValue {
			return "max_value must be greater than min_value"
		}
		if (t.MaxValue-t.MinValue)/t.Step > 20 {
			return "a scale can have at most 21 steps"
		}
	}
	if in.StartDate != "" {
		if !validDate(in.StartDate) {
			return "start_date must be YYYY-MM-DD"
		}
		t.StartDate = in.StartDate
	}
	if in.EndDate == nil || *in.EndDate == "" {
		t.EndDate = nil
	} else {
		if !validDate(*in.EndDate) {
			return "end_date must be YYYY-MM-DD"
		}
		t.EndDate = in.EndDate
	}
	return ""
}

func (s *Server) handleCreateTracker(w http.ResponseWriter, r *http.Request) {
	var in trackerInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t := &store.Tracker{Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true}
	if in.Kind == "number" {
		t.MinValue, t.MaxValue, t.Step, t.HigherIsBetter = 0, 0, 0.1, false
	}
	if msg := in.apply(t); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateTracker(r.Context(), t); err != nil {
		s.fail(w, err, "create tracker")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTracker(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTracker(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get tracker")
		return
	}
	var in trackerInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(t); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateTracker(r.Context(), t); err != nil {
		s.fail(w, err, "update tracker")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTracker(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTracker(r.Context(), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete tracker")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEndTracker(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTracker(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get tracker")
		return
	}
	var in struct {
		Date string `json:"date"`
	}
	_ = decode(r, &in)
	end := endOfMonth(in.Date)
	if end < t.StartDate {
		end = t.StartDate
	}
	t.EndDate = &end
	if err := s.store.UpdateTracker(r.Context(), t); err != nil {
		s.fail(w, err, "end tracker")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleResumeTracker(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTracker(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get tracker")
		return
	}
	t.EndDate = nil
	if err := s.store.UpdateTracker(r.Context(), t); err != nil {
		s.fail(w, err, "resume tracker")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleReorderTrackers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderTrackers(r.Context(), in.IDs); err != nil {
		s.fail(w, err, "reorder trackers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetTrackerEntry(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if date > store.Today() {
		writeErr(w, http.StatusBadRequest, "cannot record a day in the future")
		return
	}
	t, err := s.store.GetTracker(r.Context(), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get tracker")
		return
	}
	var in struct {
		Value *float64 `json:"value"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Value != nil && t.Kind == "scale" && (*in.Value < t.MinValue || *in.Value > t.MaxValue) {
		writeErr(w, http.StatusBadRequest, "value out of range")
		return
	}
	if err := s.store.SetTrackerEntry(r.Context(), t.ID, date, in.Value); err != nil {
		s.fail(w, err, "set value")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracker_id": t.ID, "date": date, "value": in.Value})
}

func (s *Server) handleClearTrackerEntry(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if err := s.store.SetTrackerEntry(r.Context(), idParam(r, "id"), date, nil); err != nil {
		s.fail(w, err, "clear value")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
