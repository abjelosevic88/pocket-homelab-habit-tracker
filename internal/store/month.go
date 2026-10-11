package store

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// Day is one column of the month page.
type Day struct {
	Date    string `json:"date"`
	Day     int    `json:"day"`
	Weekday int    `json:"weekday"` // ISO 1..7
	Label   string `json:"label"`   // M T W T F S S
	Weekend bool   `json:"weekend"`
	Today   bool   `json:"today"`
	Future  bool   `json:"future"`
}

// MonthHabit is a habit row with its entries and counts.
type MonthHabit struct {
	Habit
	Entries   map[string]int `json:"entries"`
	Done      int            `json:"done"`
	Missed    int            `json:"missed"`
	Scheduled int            `json:"scheduled"` // scheduled days up to today
	Streak    int            `json:"streak"`
}

// MonthTracker is a tracker row with its values and summary.
type MonthTracker struct {
	Tracker
	Entries map[string]float64 `json:"entries"`
	Avg     *float64           `json:"avg"`
	Min     *float64           `json:"min"`
	Max     *float64           `json:"max"`
	First   *float64           `json:"first"`
	Last    *float64           `json:"last"`
}

// MonthSummary totals a month.
type MonthSummary struct {
	Done        int      `json:"done"`
	Missed      int      `json:"missed"`
	Unmarked    int      `json:"unmarked"`
	Rate        *float64 `json:"rate"`
	PerfectDays int      `json:"perfect_days"`
	JournalDays int      `json:"journal_days"`
}

// MonthPage is everything the month view needs in one call.
type MonthPage struct {
	Month    string            `json:"month"`
	Days     []Day             `json:"days"`
	Habits   []MonthHabit      `json:"habits"`
	Trackers []MonthTracker    `json:"trackers"`
	Journal  map[string]string `json:"journal"`
	Summary  MonthSummary      `json:"summary"`
}

var weekdayLetters = map[int]string{1: "M", 2: "T", 3: "W", 4: "T", 5: "F", 6: "S", 7: "S"}

// Days builds the day columns for a month.
func Days(month string, today string) ([]Day, error) {
	first, last, err := MonthBounds(month)
	if err != nil {
		return nil, err
	}
	var out []Day
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		wd := int(d.Weekday())
		if wd == 0 {
			wd = 7
		}
		ds := d.Format("2006-01-02")
		out = append(out, Day{Date: ds, Day: d.Day(), Weekday: wd, Label: weekdayLetters[wd], Weekend: wd >= 6, Today: ds == today, Future: ds > today})
	}
	return out, nil
}

// Month assembles the month page.
func (s *Store) Month(ctx context.Context, month string) (*MonthPage, error) {
	today := Today()
	days, err := Days(month, today)
	if err != nil {
		return nil, err
	}
	from, to := days[0].Date, days[len(days)-1].Date
	habits, err := s.ListHabits(ctx, month, true)
	if err != nil {
		return nil, err
	}
	trackers, err := s.ListTrackers(ctx, month, true)
	if err != nil {
		return nil, err
	}
	hEntries, err := s.HabitEntries(ctx, from, to)
	if err != nil {
		return nil, err
	}
	tEntries, err := s.TrackerEntries(ctx, from, to)
	if err != nil {
		return nil, err
	}
	journal, err := s.Journal(ctx, from, to)
	if err != nil {
		return nil, err
	}
	page := &MonthPage{Month: month, Days: days, Journal: journal, Habits: []MonthHabit{}, Trackers: []MonthTracker{}}
	perfect := map[string]bool{}
	for _, d := range days {
		if !d.Future {
			perfect[d.Date] = true
		}
	}
	anyScheduled := map[string]bool{}
	for _, h := range habits {
		mh := MonthHabit{Habit: h, Entries: hEntries[h.ID]}
		if mh.Entries == nil {
			mh.Entries = map[string]int{}
		}
		for _, d := range days {
			t, _ := ParseDate(d.Date)
			sched := h.Scheduled(t) && h.ActiveOn(d.Date)
			st := mh.Entries[d.Date]
			switch st {
			case 1:
				mh.Done++
			case -1:
				mh.Missed++
			}
			if sched && !d.Future {
				mh.Scheduled++
				anyScheduled[d.Date] = true
				if st != 1 {
					perfect[d.Date] = false
				}
			}
		}
		page.Summary.Done += mh.Done
		page.Summary.Missed += mh.Missed
		page.Summary.Unmarked += maxInt(0, mh.Scheduled-mh.Done-mh.Missed)
		mh.Streak = s.streak(ctx, h, today)
		page.Habits = append(page.Habits, mh)
	}
	for d, ok := range perfect {
		if ok && anyScheduled[d] {
			page.Summary.PerfectDays++
		}
	}
	if n := page.Summary.Done + page.Summary.Missed; n > 0 {
		r := float64(page.Summary.Done) / float64(n)
		page.Summary.Rate = &r
	}
	page.Summary.JournalDays = len(journal)
	for _, t := range trackers {
		mt := MonthTracker{Tracker: t, Entries: tEntries[t.ID]}
		if mt.Entries == nil {
			mt.Entries = map[string]float64{}
		}
		var dates []string
		for d := range mt.Entries {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		if len(dates) > 0 {
			sum, mn, mx := 0.0, math.Inf(1), math.Inf(-1)
			for _, d := range dates {
				v := mt.Entries[d]
				sum += v
				mn = math.Min(mn, v)
				mx = math.Max(mx, v)
			}
			avg := round2(sum / float64(len(dates)))
			f, l := mt.Entries[dates[0]], mt.Entries[dates[len(dates)-1]]
			mt.Avg, mt.Min, mt.Max, mt.First, mt.Last = &avg, &mn, &mx, &f, &l
		}
		page.Trackers = append(page.Trackers, mt)
	}
	return page, nil
}

// streak counts consecutive scheduled days marked done, ending today or yesterday.
func (s *Store) streak(ctx context.Context, h Habit, today string) int {
	t, _ := ParseDate(today)
	from := t.AddDate(0, 0, -400).Format("2006-01-02")
	entries, err := s.HabitEntries(ctx, from, today)
	if err != nil {
		return 0
	}
	e := entries[h.ID]
	n := 0
	first := true
	for d := t; d.Format("2006-01-02") >= from && d.Format("2006-01-02") >= h.StartDate; d = d.AddDate(0, 0, -1) {
		ds := d.Format("2006-01-02")
		if !h.Scheduled(d) || !h.ActiveOn(ds) {
			continue
		}
		switch e[ds] {
		case 1:
			n++
		default:
			if first && e[ds] == 0 {
				// today not marked yet does not break the streak
				first = false
				continue
			}
			return n
		}
		first = false
	}
	return n
}

// TodayPage is the quick check-in payload.
type TodayPage struct {
	Date     string         `json:"date"`
	Weekday  int            `json:"weekday"`
	Habits   []TodayHabit   `json:"habits"`
	Trackers []TodayTracker `json:"trackers"`
	Journal  string         `json:"journal"`
	Done     int            `json:"done"`
	Total    int            `json:"total"`
}

// TodayHabit is one habit's state for a day.
type TodayHabit struct {
	Habit
	Status    int  `json:"status"`
	Scheduled bool `json:"scheduled"`
	Streak    int  `json:"streak"`
}

// TodayTracker is one tracker's value for a day.
type TodayTracker struct {
	Tracker
	Value    *float64 `json:"value"`
	Previous *float64 `json:"previous"` // most recent earlier value, for number trackers
}

// Day assembles the check-in page for one date.
func (s *Store) Day(ctx context.Context, date string) (*TodayPage, error) {
	t, err := ParseDate(date)
	if err != nil {
		return nil, err
	}
	month := t.Format("2006-01")
	habits, err := s.ListHabits(ctx, month, true)
	if err != nil {
		return nil, err
	}
	trackers, err := s.ListTrackers(ctx, month, true)
	if err != nil {
		return nil, err
	}
	hEntries, err := s.HabitEntries(ctx, date, date)
	if err != nil {
		return nil, err
	}
	tEntries, err := s.TrackerEntries(ctx, date, date)
	if err != nil {
		return nil, err
	}
	journal, err := s.Journal(ctx, date, date)
	if err != nil {
		return nil, err
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	page := &TodayPage{Date: date, Weekday: wd, Journal: journal[date], Habits: []TodayHabit{}, Trackers: []TodayTracker{}}
	for _, h := range habits {
		if !h.ActiveOn(date) {
			continue
		}
		th := TodayHabit{Habit: h, Status: hEntries[h.ID][date], Scheduled: h.Scheduled(t), Streak: s.streak(ctx, h, date)}
		if th.Scheduled {
			page.Total++
			if th.Status == 1 {
				page.Done++
			}
		}
		page.Habits = append(page.Habits, th)
	}
	for _, tr := range trackers {
		if !tr.ActiveOn(date) {
			continue
		}
		tt := TodayTracker{Tracker: tr}
		if v, ok := tEntries[tr.ID][date]; ok {
			tt.Value = &v
		}
		var prev float64
		if err := s.DB.QueryRowContext(ctx, `SELECT value FROM tracker_entries WHERE tracker_id = ? AND date < ? ORDER BY date DESC LIMIT 1`, tr.ID, date).Scan(&prev); err == nil {
			tt.Previous = &prev
		}
		page.Trackers = append(page.Trackers, tt)
	}
	return page, nil
}

// Dashboard is a compact status for widgets (Homepage, Shortcuts, Home Assistant).
type Dashboard struct {
	Date          string   `json:"date"`
	TodayDone     int      `json:"today_done"`
	TodayTotal    int      `json:"today_total"`
	TodayLeft     int      `json:"today_left"`
	JournalToday  bool     `json:"journal_today"`
	MonthDone     int      `json:"month_done"`
	MonthMissed   int      `json:"month_missed"`
	MonthRate     *float64 `json:"month_rate"`
	MonthRatePct  *int     `json:"month_rate_pct"`
	PerfectDays   int      `json:"perfect_days"`
	PerfectStreak int      `json:"perfect_streak"`
	BestStreak    int      `json:"best_habit_streak"`
	BestHabit     string   `json:"best_habit"`
}

// DashboardStats computes the dashboard for today.
func (s *Store) DashboardStats(ctx context.Context) (*Dashboard, error) {
	today := Today()
	day, err := s.Day(ctx, today)
	if err != nil {
		return nil, err
	}
	month, err := s.Month(ctx, today[:7])
	if err != nil {
		return nil, err
	}
	d := &Dashboard{Date: today, TodayDone: day.Done, TodayTotal: day.Total, TodayLeft: day.Total - day.Done, JournalToday: day.Journal != "",
		MonthDone: month.Summary.Done, MonthMissed: month.Summary.Missed, MonthRate: month.Summary.Rate, PerfectDays: month.Summary.PerfectDays}
	if d.MonthRate != nil {
		p := int(math.Round(*d.MonthRate * 100))
		d.MonthRatePct = &p
	}
	for _, h := range month.Habits {
		if h.Streak > d.BestStreak {
			d.BestStreak, d.BestHabit = h.Streak, h.Name
		}
	}
	d.PerfectStreak = s.perfectStreak(ctx, today)
	return d, nil
}

// perfectStreak counts consecutive days (ending today or yesterday) where every scheduled habit was done.
func (s *Store) perfectStreak(ctx context.Context, today string) int {
	t, _ := ParseDate(today)
	from := t.AddDate(0, 0, -400).Format("2006-01-02")
	habits, err := s.ListHabits(ctx, "", true)
	if err != nil {
		return 0
	}
	entries, err := s.HabitEntries(ctx, from, today)
	if err != nil {
		return 0
	}
	n := 0
	for d := t; d.Format("2006-01-02") >= from; d = d.AddDate(0, 0, -1) {
		ds := d.Format("2006-01-02")
		sched, done := 0, 0
		for _, h := range habits {
			if h.ActiveOn(ds) && h.Scheduled(d) {
				sched++
				if entries[h.ID][ds] == 1 {
					done++
				}
			}
		}
		if sched == 0 {
			if ds == today {
				continue
			}
			return n
		}
		if done == sched {
			n++
			continue
		}
		if ds == today && done == 0 && !anyMarked(habits, entries, ds) {
			continue // today untouched so far
		}
		return n
	}
	return n
}

func anyMarked(habits []Habit, entries map[int64]map[string]int, date string) bool {
	for _, h := range habits {
		if entries[h.ID][date] != 0 {
			return true
		}
	}
	return false
}

// ---------- insights ----------

// HabitInsight summarises a habit over a range.
type HabitInsight struct {
	Habit
	Done       int        `json:"done"`
	Missed     int        `json:"missed"`
	Scheduled  int        `json:"scheduled"`
	Rate       *float64   `json:"rate"`
	Streak     int        `json:"streak"`
	BestStreak int        `json:"best_streak"`
	Weekdays   []*float64 `json:"weekdays"` // completion rate per ISO weekday 1..7
}

// Correlation compares a tracker's average on days a habit was done vs missed.
type Correlation struct {
	TrackerID   int64    `json:"tracker_id"`
	TrackerName string   `json:"tracker_name"`
	HabitID     int64    `json:"habit_id"`
	HabitName   string   `json:"habit_name"`
	AvgDone     *float64 `json:"avg_done"`
	AvgMissed   *float64 `json:"avg_missed"`
	NDone       int      `json:"n_done"`
	NMissed     int      `json:"n_missed"`
	Delta       *float64 `json:"delta"`
}

// TrackerInsight summarises a tracker over a range.
type TrackerInsight struct {
	Tracker
	Count  int                `json:"count"`
	Avg    *float64           `json:"avg"`
	Min    *float64           `json:"min"`
	Max    *float64           `json:"max"`
	First  *float64           `json:"first"`
	Last   *float64           `json:"last"`
	Series []TrackerPoint     `json:"series"`
	Weekly map[string]float64 `json:"weekly"` // ISO week (YYYY-Www) -> avg
}

// TrackerPoint is one dated value.
type TrackerPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Insights is the analysis page payload.
type Insights struct {
	From         string           `json:"from"`
	To           string           `json:"to"`
	Habits       []HabitInsight   `json:"habits"`
	Trackers     []TrackerInsight `json:"trackers"`
	Correlations []Correlation    `json:"correlations"`
	Journal      []JournalLine    `json:"journal"`
}

// JournalLine is one dated journal entry.
type JournalLine struct {
	Date string `json:"date"`
	Text string `json:"text"`
}

// InsightsRange computes insights for an inclusive date range.
func (s *Store) InsightsRange(ctx context.Context, from, to string) (*Insights, error) {
	start, err := ParseDate(from)
	if err != nil {
		return nil, err
	}
	end, err := ParseDate(to)
	if err != nil {
		return nil, err
	}
	today := Today()
	if to > today {
		to = today
		end, _ = ParseDate(today)
	}
	habits, err := s.ListHabits(ctx, "", true)
	if err != nil {
		return nil, err
	}
	trackers, err := s.ListTrackers(ctx, "", true)
	if err != nil {
		return nil, err
	}
	hEntries, err := s.HabitEntries(ctx, from, to)
	if err != nil {
		return nil, err
	}
	tEntries, err := s.TrackerEntries(ctx, from, to)
	if err != nil {
		return nil, err
	}
	journal, err := s.Journal(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := &Insights{From: from, To: to, Habits: []HabitInsight{}, Trackers: []TrackerInsight{}, Correlations: []Correlation{}, Journal: []JournalLine{}}
	for _, h := range habits {
		hi := HabitInsight{Habit: h, Weekdays: make([]*float64, 7)}
		e := hEntries[h.ID]
		wdDone, wdSched := [8]int{}, [8]int{}
		run, best := 0, 0
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			if !h.ActiveOn(ds) || !h.Scheduled(d) {
				continue
			}
			wd := int(d.Weekday())
			if wd == 0 {
				wd = 7
			}
			hi.Scheduled++
			wdSched[wd]++
			switch e[ds] {
			case 1:
				hi.Done++
				wdDone[wd]++
				run++
				if run > best {
					best = run
				}
			case -1:
				hi.Missed++
				run = 0
			default:
				if ds != today {
					run = 0
				}
			}
		}
		for wd := 1; wd <= 7; wd++ {
			if wdSched[wd] > 0 {
				r := float64(wdDone[wd]) / float64(wdSched[wd])
				hi.Weekdays[wd-1] = &r
			}
		}
		if n := hi.Done + hi.Missed; n > 0 {
			r := float64(hi.Done) / float64(n)
			hi.Rate = &r
		}
		hi.BestStreak = best
		hi.Streak = s.streak(ctx, h, today)
		out.Habits = append(out.Habits, hi)
	}
	for _, t := range trackers {
		ti := TrackerInsight{Tracker: t, Series: []TrackerPoint{}, Weekly: map[string]float64{}}
		e := tEntries[t.ID]
		var dates []string
		for d := range e {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		sum, mn, mx := 0.0, math.Inf(1), math.Inf(-1)
		wkSum, wkN := map[string]float64{}, map[string]int{}
		for _, d := range dates {
			v := e[d]
			ti.Series = append(ti.Series, TrackerPoint{Date: d, Value: v})
			sum += v
			mn, mx = math.Min(mn, v), math.Max(mx, v)
			td, _ := ParseDate(d)
			y, w := td.ISOWeek()
			k := isoWeekKey(y, w)
			wkSum[k] += v
			wkN[k]++
		}
		if len(dates) > 0 {
			ti.Count = len(dates)
			avg := round2(sum / float64(len(dates)))
			f, l := e[dates[0]], e[dates[len(dates)-1]]
			ti.Avg, ti.Min, ti.Max, ti.First, ti.Last = &avg, &mn, &mx, &f, &l
			for k := range wkSum {
				ti.Weekly[k] = round2(wkSum[k] / float64(wkN[k]))
			}
		}
		out.Trackers = append(out.Trackers, ti)
		if t.Kind != "scale" {
			continue
		}
		for _, h := range habits {
			c := Correlation{TrackerID: t.ID, TrackerName: t.Name, HabitID: h.ID, HabitName: h.Name}
			var sd, sm float64
			for d, v := range e {
				switch hEntries[h.ID][d] {
				case 1:
					sd += v
					c.NDone++
				case -1:
					sm += v
					c.NMissed++
				}
			}
			if c.NDone > 0 {
				a := round2(sd / float64(c.NDone))
				c.AvgDone = &a
			}
			if c.NMissed > 0 {
				a := round2(sm / float64(c.NMissed))
				c.AvgMissed = &a
			}
			if c.AvgDone != nil && c.AvgMissed != nil {
				dlt := round2(*c.AvgDone - *c.AvgMissed)
				c.Delta = &dlt
			}
			if c.NDone+c.NMissed >= 2 {
				out.Correlations = append(out.Correlations, c)
			}
		}
	}
	sort.SliceStable(out.Correlations, func(i, j int) bool {
		di, dj := out.Correlations[i].Delta, out.Correlations[j].Delta
		if di == nil {
			return false
		}
		if dj == nil {
			return true
		}
		return math.Abs(*di) > math.Abs(*dj)
	})
	var jd []string
	for d := range journal {
		jd = append(jd, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(jd)))
	for _, d := range jd {
		out.Journal = append(out.Journal, JournalLine{Date: d, Text: journal[d]})
	}
	return out, nil
}

func isoWeekKey(y, w int) string { return fmt.Sprintf("%04d-W%02d", y, w) }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
