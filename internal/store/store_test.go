package store

import (
	"context"
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

func TestHabitScheduleAndMonth(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	today := Today()
	month := today[:7]
	first, _, _ := MonthBounds(month)
	h := &Habit{Name: "Run", Days: "135", StartDate: first.Format("2006-01-02")}
	if err := s.CreateHabit(ctx, h); err != nil {
		t.Fatal(err)
	}
	if h.ID == 0 {
		t.Fatal("expected id")
	}
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // a Monday
	if !h.Scheduled(mon) || h.Scheduled(mon.AddDate(0, 0, 1)) {
		t.Fatalf("schedule wrong: mon=%v tue=%v", h.Scheduled(mon), h.Scheduled(mon.AddDate(0, 0, 1)))
	}
	if err := s.SetHabitEntry(ctx, h.ID, today, 1); err != nil {
		t.Fatal(err)
	}
	page, err := s.Month(ctx, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Habits) != 1 || page.Habits[0].Done != 1 || page.Habits[0].Entries[today] != 1 {
		t.Fatalf("month page wrong: %+v", page.Habits)
	}
	if err := s.SetHabitEntry(ctx, h.ID, today, 0); err != nil {
		t.Fatal(err)
	}
	page, _ = s.Month(ctx, month)
	if page.Habits[0].Done != 0 {
		t.Fatal("clear did not remove entry")
	}
	// ending a habit hides it from later months but keeps it on this one
	end := today
	h.EndDate = &end
	if err := s.UpdateHabit(ctx, h); err != nil {
		t.Fatal(err)
	}
	later := first.AddDate(0, 2, 0).Format("2006-01")
	if hs, _ := s.ListHabits(ctx, later, true); len(hs) != 0 {
		t.Fatalf("ended habit still listed for %s", later)
	}
	if hs, _ := s.ListHabits(ctx, month, true); len(hs) != 1 {
		t.Fatal("ended habit missing from its own month")
	}
}

func TestStreakSkipsUnscheduledAndUnmarkedToday(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	today, _ := ParseDate(Today())
	h := &Habit{Name: "Daily", Days: "1234567", StartDate: today.AddDate(0, 0, -30).Format("2006-01-02")}
	if err := s.CreateHabit(ctx, h); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		_ = s.SetHabitEntry(ctx, h.ID, today.AddDate(0, 0, -i).Format("2006-01-02"), 1)
	}
	_ = s.SetHabitEntry(ctx, h.ID, today.AddDate(0, 0, -5).Format("2006-01-02"), -1)
	if n := s.streak(ctx, *h, Today()); n != 4 {
		t.Fatalf("streak = %d, want 4 (today unmarked must not break it)", n)
	}
	_ = s.SetHabitEntry(ctx, h.ID, Today(), 1)
	if n := s.streak(ctx, *h, Today()); n != 5 {
		t.Fatalf("streak = %d, want 5", n)
	}
}

func TestTrackersJournalAndInsights(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	today, _ := ParseDate(Today())
	start := today.AddDate(0, 0, -10).Format("2006-01-02")
	h := &Habit{Name: "Run", StartDate: start}
	if err := s.CreateHabit(ctx, h); err != nil {
		t.Fatal(err)
	}
	tr := &Tracker{Name: "Day rating", Kind: "scale", MinValue: 1, MaxValue: 5, Step: 1, HigherIsBetter: true, StartDate: start}
	if err := s.CreateTracker(ctx, tr); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		v := 2.0
		st := -1
		if i%2 == 0 {
			v, st = 4, 1
		}
		_ = s.SetHabitEntry(ctx, h.ID, d, st)
		_ = s.SetTrackerEntry(ctx, tr.ID, d, &v)
	}
	_ = s.SetJournal(ctx, Today(), "good day", nil)
	_ = s.SetJournalBad(ctx, Today(), "rain")
	ins, err := s.InsightsRange(ctx, start, Today())
	if err != nil {
		t.Fatal(err)
	}
	if len(ins.Correlations) != 1 || ins.Correlations[0].Delta == nil || *ins.Correlations[0].Delta != 2 {
		t.Fatalf("correlation wrong: %+v", ins.Correlations)
	}
	if len(ins.Journal) != 1 || ins.Journal[0].Text != "good day" || ins.Journal[0].Bad != "rain" {
		t.Fatalf("journal wrong: %+v", ins.Journal)
	}
	d, err := s.DashboardStats(ctx)
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
