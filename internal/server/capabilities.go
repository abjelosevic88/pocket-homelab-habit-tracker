package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Habits (the capabilities
// value in links.go): /api/v1/pocket/search, /day/{date}, /widget and
// /capture, in the shape every Pocket app answers in. Day and widget read the
// day GET /days/{date} reads, as the request's person; only the capture
// writes, one line added to a day's journal.

// dayURL opens a day's check-in page, habitURL a habit on the Habits &
// trackers page (where the command palette and the month page send it).
func dayURL(date string) string { return "/today/" + date }
func habitURL(id int64) string  { return "/manage?habit=" + strconv.FormatInt(id, 10) }

// habitRef names a habit where an answer also has journal lines.
func habitRef(id int64) string { return "habit:" + strconv.FormatInt(id, 10) }

// dayWords writes a day for a person: "12 Nov 2026".
func dayWords(date string) string {
	t, err := store.ParseDate(date)
	if err != nil {
		return date
	}
	return t.Format("2 Jan 2006")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// streakWords is a run of days worth a mention, as the flame on the check-in
// page: "5-day streak", nothing for a single day.
func streakWords(n int) string {
	if n > 1 {
		return strconv.Itoa(n) + "-day streak"
	}
	return ""
}

var weekdayShort = [...]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// scheduleWords says when a habit is planned, as the Habits & trackers page
// does: "Every day · since 1 Oct 2026", "Mon Wed Fri · since … · until …".
func scheduleWords(h store.Habit) string {
	var days string
	switch h.Days {
	case "1234567":
		days = "Every day"
	case "12345":
		days = "Weekdays"
	case "67":
		days = "Weekends"
	default:
		var names []string
		for _, d := range h.Days {
			if d >= '1' && d <= '7' {
				names = append(names, weekdayShort[d-'1'])
			}
		}
		days = strings.Join(names, " ")
	}
	out := days + " · since " + dayWords(h.StartDate)
	if h.EndDate != nil {
		out += " · until " + dayWords(*h.EndDate)
	}
	return out
}

// ---------- search ----------

// pocketSearch answers Pocket Search: the person's habits whose name has the
// words (the ones still running first), then the journal days that have them
// in the good line, the bad line or the important note, newest first.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx, uid := r.Context(), me(r)
	habits, err := s.store.SearchHabits(ctx, uid, q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]core.SearchItem, 0, limit)
	for _, h := range habits {
		out = append(out, core.SearchItem{ID: habitRef(h.ID), Title: h.Name, Snippet: scheduleWords(h), Kind: "habit", Date: h.StartDate, URL: habitURL(h.ID)})
	}
	if room := limit - len(out); room > 0 {
		days, err := s.store.SearchJournal(ctx, uid, q, room)
		if err != nil {
			return nil, err
		}
		for _, d := range days {
			out = append(out, core.SearchItem{ID: "journal:" + d.Date, Title: "Journal, " + dayWords(d.Date), Snippet: matchingLine(q, d.Text, d.Bad, d.Important),
				Kind: "journal", Date: d.Date, URL: dayURL(d.Date)})
		}
	}
	return out, nil
}

// matchingLine picks, from a day's journal, the line that has the most of the
// words. A very long line starts shortly before the first word found, so the
// match is still there after the answer is cut to length.
func matchingLine(q string, fields ...string) string {
	words := strings.Fields(strings.ToLower(q))
	best, bestLow, most := "", "", -1
	for _, f := range fields {
		for _, line := range strings.Split(f, "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			low, n := strings.ToLower(line), 0
			for _, w := range words {
				if strings.Contains(low, w) {
					n++
				}
			}
			if n > most {
				best, bestLow, most = line, low, n
			}
		}
	}
	const keep, before = 240, 60
	line := []rune(best)
	if len(line) <= keep || len([]rune(bestLow)) != len(line) {
		return best
	}
	for _, w := range words {
		if i := strings.Index(bestLow, w); i >= 0 {
			if at := utf8.RuneCountInString(bestLow[:i]); at > keep {
				return "…" + strings.TrimSpace(string(line[at-before:]))
			}
			break
		}
	}
	return best
}

// ---------- day ----------

// pocketDay answers Pocket Day with the day GET /days/{date} gives: one item
// for each habit planned that day (done, missed or still open), then what was
// written in the journal. Days here are plain dates; the caller's time zone
// only decides whether the day is still to come.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx, uid := r.Context(), me(r)
	page, err := s.store.Day(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	today := store.Today()
	if tz != nil {
		today = time.Now().In(tz).Format("2006-01-02")
	}
	day := &core.Day{Date: date}
	for _, h := range page.Habits {
		if !h.Scheduled {
			continue
		}
		it := core.DayItem{ID: habitRef(h.ID), Title: h.Name, Kind: "open", URL: dayURL(date)}
		switch h.Status {
		case 1:
			it.Kind, it.Done, it.Sub = "done", true, streakWords(h.Streak)
		case -1:
			it.Kind = "missed"
		}
		day.Items = append(day.Items, it)
	}
	var parts []string
	switch {
	case page.Total == 0:
	case date > today:
		parts = append(parts, plural(page.Total, "habit", "habits")+" planned")
	default:
		done, habits := fmt.Sprintf("%d of %d", page.Done, page.Total), "habits"
		if page.Total == 1 {
			habits = "habit"
		}
		parts = append(parts, done+" "+habits+" done")
		day.Metrics = append(day.Metrics, core.Metric{Label: "Done", Value: done})
	}

	// the journal under the person's own headings
	st, _ := s.store.GetSettings(ctx, uid)
	for _, l := range []struct{ id, title, text string }{
		{"journal:" + date, st.JournalPrompt, page.Journal},
		{"journal-bad:" + date, st.BadPrompt, page.JournalBad},
		{"important:" + date, "Important", page.Important},
	} {
		if strings.TrimSpace(l.text) != "" {
			day.Items = append(day.Items, core.DayItem{ID: l.id, Title: l.title, Kind: "journal", Note: strings.TrimSpace(l.text), URL: dayURL(date)})
		}
	}
	if strings.TrimSpace(page.Journal+page.JournalBad) != "" {
		parts = append(parts, "journal written")
	}
	if strings.TrimSpace(page.Important) != "" {
		parts = append(parts, "an important note")
	}
	if len(day.Items) == 0 {
		return nil, nil
	}
	day.Summary = upperFirst(strings.Join(parts, ", "))
	return day, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub with today as the check-in page has it: the
// habits planned today, the ones still to mark first, then the missed ones,
// then the done ones.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	page, err := s.store.Day(r.Context(), me(r), store.Today())
	if err != nil {
		return nil, err
	}
	w := &core.WidgetData{URL: "/today", Metrics: []core.Metric{
		// Pocket Hub's phone apps draw the habit ring from these two, found by
		// name: whole numbers, the day's done and total, in this order.
		{Label: "Done", Value: strconv.Itoa(page.Done)},
		{Label: "Scheduled", Value: strconv.Itoa(page.Total)},
	}}
	var open, missed, done []core.WidgetItem
	for _, h := range page.Habits {
		if !h.Scheduled {
			continue
		}
		it := core.WidgetItem{ID: strconv.FormatInt(h.ID, 10), Title: h.Name, Detail: streakWords(h.Streak), URL: "/today"}
		switch h.Status {
		case 1:
			it.Done, it.Detail = true, firstNonEmpty(it.Detail, "Done")
			done = append(done, it)
		case -1:
			it.Detail = "Missed"
			missed = append(missed, it)
		default:
			open = append(open, it)
		}
	}
	w.Items = append(append(open, missed...), done...)
	switch {
	case page.Total == 0:
		// someone with no habits yet gets no line at all
		if len(page.Habits) > 0 {
			w.Line = "Nothing planned today"
		}
	case len(open) > 0:
		w.Line = strconv.Itoa(len(open)) + " left today"
	case page.Done == page.Total:
		w.Line = "All done today"
	default:
		w.Line = fmt.Sprintf("%d of %d done today", page.Done, page.Total)
	}
	return w, nil
}

// ---------- capture ----------

// pocketCapture keeps a quick line in the journal of today, or of the day
// given: on a line of its own under what is already written there, which
// stays as it is (POST /checkin and PUT /journal replace the text; this never
// does). The same request_id again answers the same and adds nothing: the
// receipt is written in the transaction that adds the line. Like a check-in
// it publishes habits.checkin, naming the app that sent it.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx, uid := r.Context(), me(r)
	op, err := core.NewOperation(fmt.Sprintf("habits:user:%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	var res core.CaptureResult
	replay := func() (*core.CaptureResult, error) {
		found, err := op.Replay(ctx, s.store.DB, &res)
		switch {
		case errors.Is(err, core.ErrOperationConflict):
			return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
		case err != nil || !found:
			return nil, err
		}
		return &res, nil
	}
	if kept, err := replay(); kept != nil || err != nil {
		return kept, err
	}

	date := firstNonEmpty(in.Date, store.Today())
	if date > store.Today() {
		return nil, fmt.Errorf("%w: the journal is for today or a day that has passed", core.ErrCapture)
	}
	line := in.Text
	switch {
	case line == "":
		line = in.Title
	case in.Title != "":
		line = in.Title + ": " + line
	}
	line = strings.TrimSpace(strings.ReplaceAll(line, "\r\n", "\n"))
	if len(line) > store.JournalMax {
		return nil, fmt.Errorf("%w: keep it under %d characters", core.ErrCapture, store.JournalMax)
	}
	title := strings.Join(strings.Fields(line), " ")
	if t := []rune(title); len(t) > 300 {
		title = strings.TrimSpace(string(t[:299])) + "…"
	}
	kept := core.CaptureResult{ID: date, Title: title, URL: dayURL(date)}
	_, err = s.store.AppendJournal(ctx, uid, date, line,
		func(tx *sql.Tx) error { return op.Save(ctx, tx, kept) },
		s.userEvent(ctx, userFrom(ctx), "habits.checkin", "day:"+date, viaLink(r, map[string]any{"date": date, "habits": 0, "trackers": 0, "journal": true})))
	if err != nil {
		// the same line sent twice at once: the other one kept it
		if was, rerr := replay(); was != nil || rerr != nil {
			return was, rerr
		}
		if errors.Is(err, store.ErrJournalFull) {
			return nil, fmt.Errorf("%w: that day's journal is full, shorten it in Pocket Habits first", core.ErrCapture)
		}
		return nil, err
	}
	return &kept, nil
}
