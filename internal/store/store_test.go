package store

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mkUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	u := &User{Email: email, Name: email, Role: "admin"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestHabitScheduleAndMonth(t *testing.T) {
	s := openTest(t)
	uid := mkUser(t, s, "owner@example.com")
	ctx := context.Background()
	today := Today()
	month := today[:7]
	first, _, _ := MonthBounds(month)
	h := &Habit{Name: "Run", Days: "135", StartDate: first.Format("2006-01-02")}
	if err := s.CreateHabit(ctx, uid, h); err != nil {
		t.Fatal(err)
	}
	if h.ID == 0 {
		t.Fatal("expected id")
	}
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // a Monday
	if !h.Scheduled(mon) || h.Scheduled(mon.AddDate(0, 0, 1)) {
		t.Fatalf("schedule wrong: mon=%v tue=%v", h.Scheduled(mon), h.Scheduled(mon.AddDate(0, 0, 1)))
	}
	if err := s.SetHabitEntry(ctx, uid, h.ID, today, 1); err != nil {
		t.Fatal(err)
	}
	page, err := s.Month(ctx, uid, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Habits) != 1 || page.Habits[0].Done != 1 || page.Habits[0].Entries[today] != 1 {
		t.Fatalf("month page wrong: %+v", page.Habits)
	}
	if err := s.SetHabitEntry(ctx, uid, h.ID, today, 0); err != nil {
		t.Fatal(err)
	}
	page, _ = s.Month(ctx, uid, month)
	if page.Habits[0].Done != 0 {
		t.Fatal("clear did not remove entry")
	}
	// ending a habit hides it from later months but keeps it on this one
	end := today
	h.EndDate = &end
	if err := s.UpdateHabit(ctx, uid, h); err != nil {
		t.Fatal(err)
	}
	later := first.AddDate(0, 2, 0).Format("2006-01")
	if hs, _ := s.ListHabits(ctx, uid, later, true); len(hs) != 0 {
		t.Fatalf("ended habit still listed for %s", later)
	}
	if hs, _ := s.ListHabits(ctx, uid, month, true); len(hs) != 1 {
		t.Fatal("ended habit missing from its own month")
	}
}

func TestStreakSkipsUnscheduledAndUnmarkedToday(t *testing.T) {
	s := openTest(t)
	uid := mkUser(t, s, "owner@example.com")
	ctx := context.Background()
	today, _ := ParseDate(Today())
	h := &Habit{Name: "Daily", Days: "1234567", StartDate: today.AddDate(0, 0, -30).Format("2006-01-02")}
	if err := s.CreateHabit(ctx, uid, h); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		_ = s.SetHabitEntry(ctx, uid, h.ID, today.AddDate(0, 0, -i).Format("2006-01-02"), 1)
	}
	_ = s.SetHabitEntry(ctx, uid, h.ID, today.AddDate(0, 0, -5).Format("2006-01-02"), -1)
	if n := s.streak(ctx, uid, *h, Today()); n != 4 {
		t.Fatalf("streak = %d, want 4 (today unmarked must not break it)", n)
	}
	_ = s.SetHabitEntry(ctx, uid, h.ID, Today(), 1)
	if n := s.streak(ctx, uid, *h, Today()); n != 5 {
		t.Fatalf("streak = %d, want 5", n)
	}
}

func TestTrackersJournalAndInsights(t *testing.T) {
	s := openTest(t)
	uid := mkUser(t, s, "owner@example.com")
	ctx := context.Background()
	today, _ := ParseDate(Today())
	start := today.AddDate(0, 0, -10).Format("2006-01-02")
	h := &Habit{Name: "Run", StartDate: start}
	if err := s.CreateHabit(ctx, uid, h); err != nil {
		t.Fatal(err)
	}
	tr := &Tracker{Name: "Day rating", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true, StartDate: start}
	if err := s.CreateTracker(ctx, uid, tr); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		v := 2.0
		st := -1
		if i%2 == 0 {
			v, st = 4, 1
		}
		_ = s.SetHabitEntry(ctx, uid, h.ID, d, st)
		_ = s.SetTrackerEntry(ctx, uid, tr.ID, d, &v)
	}
	_ = s.SetJournal(ctx, uid, Today(), "good day", nil)
	_ = s.SetJournalBad(ctx, uid, Today(), "rain")
	ins, err := s.InsightsRange(ctx, uid, start, Today())
	if err != nil {
		t.Fatal(err)
	}
	if len(ins.Correlations) != 1 || ins.Correlations[0].Delta == nil || *ins.Correlations[0].Delta != 2 {
		t.Fatalf("correlation wrong: %+v", ins.Correlations)
	}
	if len(ins.Journal) != 1 || ins.Journal[0].Text != "good day" || ins.Journal[0].Bad != "rain" {
		t.Fatalf("journal wrong: %+v", ins.Journal)
	}
	d, err := s.DashboardStats(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if d.TodayDone != 1 || d.TodayTotal != 1 || !d.JournalToday {
		t.Fatalf("dashboard wrong: %+v", d)
	}
	// Backup produces a readable copy
	dst := filepath.Join(t.TempDir(), "copy.db")
	if err := s.Backup(ctx, dst); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Size() == 0 {
		t.Fatal("backup empty")
	}
}

func TestTrackerDefaultFillsPastDaysOnly(t *testing.T) {
	s := openTest(t)
	uid := mkUser(t, s, "owner@example.com")
	ctx := context.Background()
	today := Today()
	tt, _ := ParseDate(today)
	start := tt.AddDate(0, 0, -3).Format("2006-01-02")
	no := 0.0
	tr := &Tracker{Name: "Stomach?", Kind: "yesno", MinValue: 0, MaxValue: 1, Step: 1, StartDate: start, DefaultValue: &no}
	if err := s.CreateTracker(ctx, uid, tr); err != nil {
		t.Fatal(err)
	}
	yes := 1.0
	yesterday := tt.AddDate(0, 0, -1).Format("2006-01-02")
	if err := s.SetTrackerEntry(ctx, uid, tr.ID, yesterday, &yes); err != nil {
		t.Fatal(err)
	}
	from, to := tt.AddDate(0, 0, -5).Format("2006-01-02"), tt.AddDate(0, 0, 2).Format("2006-01-02")
	entries, _ := s.TrackerEntries(ctx, uid, from, to)
	filled := FillDefaults([]Tracker{*tr}, entries, from, to, today)
	e := entries[tr.ID]
	if len(e) != 4 { // start..today = 4 days; before start and future are untouched
		t.Fatalf("expected 4 values, got %d: %v", len(e), e)
	}
	if e[yesterday] != 1 || filled[tr.ID][yesterday] {
		t.Fatalf("explicit entry must win: %v %v", e[yesterday], filled[tr.ID][yesterday])
	}
	if e[today] != 0 || !filled[tr.ID][today] {
		t.Fatalf("today should be defaulted to no: %v %v", e[today], filled[tr.ID])
	}
	if _, ok := e[tt.AddDate(0, 0, 1).Format("2006-01-02")]; ok {
		t.Fatal("future days must not be filled")
	}
	got, _ := s.GetTracker(ctx, uid, tr.ID)
	if got.DefaultValue == nil || *got.DefaultValue != 0 {
		t.Fatalf("default not persisted: %v", got.DefaultValue)
	}
	day, err := s.Day(ctx, uid, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Trackers) == 0 || day.Trackers[len(day.Trackers)-1].Value == nil || !day.Trackers[len(day.Trackers)-1].Defaulted {
		t.Fatalf("Day() should expose the defaulted value: %+v", day.Trackers)
	}
}

// legacyDB builds a database as v0.1.2 left it (migrations 0001–0006, no owners) with an
// owner, an auto-provisioned second account and some shared-looking content.
func legacyDB(t *testing.T, withUsers bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_init.sql", "0002_journal_bad.sql", "0003_tracker_yesno.sql", "0004_weather.sql", "0005_tracker_default.sql", "0006_journal_important.sql"} {
		body, _ := migrationFS.ReadFile("migrations/" + name)
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		db.Exec(`INSERT INTO schema_migrations VALUES (?, 'x')`, name)
	}
	stmts := []string{
		`INSERT INTO habits (id, name, start_date) VALUES (1, 'Run', '2026-10-01'), (2, 'Read', '2026-10-01')`,
		`INSERT INTO trackers (id, name, kind, start_date) VALUES (1, 'Mood', 'scale', '2026-10-01')`,
		`INSERT INTO habit_entries (habit_id, date, status) VALUES (1, '2026-10-02', 1), (2, '2026-10-02', -1)`,
		`INSERT INTO tracker_entries (tracker_id, date, value) VALUES (1, '2026-10-02', 4)`,
		`INSERT INTO journal (date, text, bad_text, important) VALUES ('2026-10-02', 'good', 'bad', 'remember')`,
		`INSERT INTO weather_days (date, code, condition) VALUES ('2026-10-02', 61, 'rain')`,
		`INSERT INTO location_days (date, lat, lon) VALUES ('2026-10-02', 44.7, 17.2)`,
		`INSERT INTO settings (key, value) VALUES ('title', 'Box'), ('setup_complete', '1'), ('defaults_seeded', '1'), ('reminder_url', 'https://ntfy.example/t'), ('weather_lat', '44.7')`,
	}
	if withUsers {
		stmts = append([]string{
			`INSERT INTO users (id, email, name, password_hash, role) VALUES (1, 'owner@example.com', 'Owner', 'hash', 'admin'), (2, 'sani@example.com', 'Sani', '', 'admin')`,
		}, stmts...)
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return path
}

func TestMigrationBackfillsOwner(t *testing.T) {
	path := legacyDB(t, true)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s, err := Open(path, log)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tbl := range contentTables {
		var total, owned int
		s.DB.QueryRow(`SELECT COUNT(1), COUNT(CASE WHEN user_id = 1 THEN 1 END) FROM `+tbl).Scan(&total, &owned)
		if total == 0 || owned != total {
			t.Errorf("%s: %d of %d rows owned by the owner", tbl, owned, total)
		}
	}
	if hs, _ := s.ListHabits(ctx, 1, "", true); len(hs) != 2 {
		t.Errorf("owner habits %d", len(hs))
	}
	if hs, _ := s.ListHabits(ctx, 2, "", true); len(hs) != 0 {
		t.Errorf("second account sees %d habits", len(hs))
	}
	if j, _ := s.JournalFull(ctx, 2, "2000-01-01", "2100-01-01"); len(j) != 0 {
		t.Errorf("second account sees the journal: %v", j)
	}
	own, _ := s.GetSettings(ctx, 1)
	other, _ := s.GetSettings(ctx, 2)
	if own.ReminderURL != "https://ntfy.example/t" || own.WeatherLat != 44.7 || !own.DefaultsSeeded || own.Title != "Box" {
		t.Errorf("owner settings: %+v", own)
	}
	if other.ReminderURL != "" || other.WeatherLat != 0 || other.DefaultsSeeded || other.Title != "Box" {
		t.Errorf("second account inherited personal settings: %+v", other)
	}
	sani, _ := s.GetUserByEmail(ctx, "sani@example.com")
	if sani.Role != "member" {
		t.Errorf("auto-provisioned account is still %q", sani.Role)
	}
	// both people can now write the same day without colliding
	if err := s.SetJournal(ctx, 2, "2026-10-02", "sani's day", nil); err != nil {
		t.Fatal(err)
	}
	if j, _ := s.JournalFull(ctx, 1, "2026-10-02", "2026-10-02"); j["2026-10-02"].Text != "good" {
		t.Errorf("owner's journal overwritten: %v", j)
	}
	s.Close()
	// reopening applies nothing twice
	s, err = Open(path, log)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	if hs, _ := s.ListHabits(ctx, 1, "", true); len(hs) != 2 {
		t.Errorf("after reopen owner habits %d", len(hs))
	}
}

// TestMigrationWithoutAccountsClaimsOnFirstUser: a box upgraded before anyone signed in
// (auth mode none/proxy) keeps its rows ownerless until the first account takes them.
func TestMigrationWithoutAccountsClaimsOnFirstUser(t *testing.T) {
	path := legacyDB(t, false)
	s, err := Open(path, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	first := mkUser(t, s, "first@example.com")
	second := mkUser(t, s, "second@example.com")
	for _, tbl := range contentTables {
		var orphans int
		s.DB.QueryRow(`SELECT COUNT(1) FROM `+tbl+` WHERE user_id IS NULL OR user_id <> ?`, first).Scan(&orphans)
		if orphans != 0 {
			t.Errorf("%s: %d rows not claimed by the first account", tbl, orphans)
		}
	}
	if st, _ := s.GetSettings(ctx, first); st.ReminderURL != "https://ntfy.example/t" {
		t.Errorf("first account did not get the settings: %+v", st)
	}
	if st, _ := s.GetSettings(ctx, second); st.ReminderURL != "" {
		t.Errorf("second account got settings: %+v", st)
	}
}

func TestStoreScopesEveryQuery(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := mkUser(t, s, "a@example.com")
	b := mkUser(t, s, "b@example.com")
	today := Today()
	h := &Habit{Name: "A's habit"}
	tr := &Tracker{Name: "A's tracker", Kind: "number", Step: 1}
	if err := s.CreateHabit(ctx, a, h); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTracker(ctx, a, tr); err != nil {
		t.Fatal(err)
	}
	v := 5.0
	_ = s.SetHabitEntry(ctx, a, h.ID, today, 1)
	_ = s.SetTrackerEntry(ctx, a, tr.ID, today, &v)
	_ = s.SetJournal(ctx, a, today, "a's line", nil)
	_ = s.SetWeatherOverride(ctx, a, today, "rain")
	_ = s.SetLocation(ctx, a, LocationDay{Date: today, Lat: 1, Lon: 1})

	if _, err := s.GetHabit(ctx, b, h.ID); err != ErrNotFound {
		t.Errorf("GetHabit: %v", err)
	}
	if _, err := s.GetTracker(ctx, b, tr.ID); err != ErrNotFound {
		t.Errorf("GetTracker: %v", err)
	}
	if err := s.UpdateHabit(ctx, b, h); err != ErrNotFound {
		t.Errorf("UpdateHabit: %v", err)
	}
	if err := s.UpdateTracker(ctx, b, tr); err != ErrNotFound {
		t.Errorf("UpdateTracker: %v", err)
	}
	if err := s.SetHabitEntry(ctx, b, h.ID, today, -1); err != ErrNotFound {
		t.Errorf("SetHabitEntry: %v", err)
	}
	if err := s.SetTrackerEntry(ctx, b, tr.ID, today, nil); err != ErrNotFound {
		t.Errorf("SetTrackerEntry: %v", err)
	}
	if err := s.DeleteHabit(ctx, b, h.ID); err != ErrNotFound {
		t.Errorf("DeleteHabit: %v", err)
	}
	if err := s.DeleteTracker(ctx, b, tr.ID); err != ErrNotFound {
		t.Errorf("DeleteTracker: %v", err)
	}
	_ = s.ReorderHabits(ctx, b, []int64{h.ID})
	if e, _ := s.HabitEntries(ctx, b, "2000-01-01", today); len(e) != 0 {
		t.Errorf("HabitEntries: %v", e)
	}
	if e, _ := s.TrackerEntries(ctx, b, "2000-01-01", today); len(e) != 0 {
		t.Errorf("TrackerEntries: %v", e)
	}
	if w, _ := s.Weather(ctx, b, "2000-01-01", today); len(w) != 0 {
		t.Errorf("Weather: %v", w)
	}
	if l, _ := s.Locations(ctx, b, "2000-01-01", today); len(l) != 0 {
		t.Errorf("Locations: %v", l)
	}
	if _, err := s.FirstTrackedDay(ctx, b); err == nil {
		t.Errorf("FirstTrackedDay sees A's habits")
	}
	if c := s.Counts(ctx, b); c["habits"] != 0 || c["journal"] != 0 {
		t.Errorf("Counts: %v", c)
	}
	if d, _ := s.Day(ctx, b, today); len(d.Habits) != 0 || d.Journal != "" || d.Weather != nil {
		t.Errorf("Day: %+v", d)
	}
	// A is untouched
	got, err := s.GetHabit(ctx, a, h.ID)
	if err != nil || got.SortOrder != h.SortOrder {
		t.Errorf("A's habit: %v %+v", err, got)
	}
	if d, _ := s.Day(ctx, a, today); len(d.Habits) != 1 || d.Habits[0].Status != 1 || d.Journal != "a's line" || d.Weather == nil {
		t.Errorf("A's day: %+v", d)
	}
}
