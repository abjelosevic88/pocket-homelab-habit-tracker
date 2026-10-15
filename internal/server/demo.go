package server

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

// seedDemo fills an empty database with the notebook's seven habits and
// ~70 days of plausible history so screenshots and first impressions work.
func (s *Server) seedDemo(ctx context.Context) error {
	if habits, _ := s.store.ListHabits(ctx, "", true); len(habits) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	start := time.Now().AddDate(0, 0, -70)
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.Local)
	sd := start.Format("2006-01-02")
	type seed struct {
		name  string
		days  string
		p     float64
		color string
	}
	seeds := []seed{
		{"Run", "135", 0.7, "#2563eb"},
		{"Workout", "246", 0.75, "#7c3aed"},
		{"Morning page", "1234567", 0.8, "#0891b2"},
		{"Stretch", "1234567", 0.6, "#059669"},
		{"Brush the dog", "1234567", 0.85, "#d97706"},
		{"House renovation", "67", 0.5, "#b45309"},
		{"No alcohol", "1234567", 0.78, "#dc2626"},
	}
	var habits []store.Habit
	for _, sd0 := range seeds {
		h := store.Habit{Name: sd0.name, Days: sd0.days, Color: sd0.color, StartDate: sd}
		if err := s.store.CreateHabit(ctx, &h); err != nil {
			return err
		}
		habits = append(habits, h)
	}
	headache := store.Tracker{Name: "Headache", Kind: "yesno", MinValue: 0, MaxValue: 1, Step: 1, HigherIsBetter: false, StartDate: sd}
	if err := s.store.CreateTracker(ctx, &headache); err != nil {
		return err
	}
	trackers, err := s.store.ListTrackers(ctx, "", true)
	if err != nil {
		return err
	}
	for i := range trackers {
		trackers[i].StartDate = sd
		_ = s.store.UpdateTracker(ctx, &trackers[i])
	}
	today := time.Now()
	weight := 82.0
	lines := []string{
		"Finished the kitchen plan.", "Long call with mum.", "Best run in weeks.", "Quiet evening, early night.",
		"Boss back from holiday — pressure off.", "Fixed the leaking tap myself.", "Dinner with friends.", "Read for an hour.",
		"Sun was out all day.", "Closed three tickets at work.", "Dog learned a new trick.", "Walked to work.",
	}
	important := []string{"Started the new job.", "Doctor: blood tests came back fine.", "Signed the flat contract.", "Anna's birthday party."}
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		doneCount := 0
		for i, h := range habits {
			if !h.Scheduled(d) {
				continue
			}
			if ds == today.Format("2006-01-02") && rng.Float64() < 0.5 {
				continue
			}
			if rng.Float64() < seeds[i].p {
				_ = s.store.SetHabitEntry(ctx, h.ID, ds, 1)
				doneCount++
			} else {
				_ = s.store.SetHabitEntry(ctx, h.ID, ds, -1)
			}
		}
		if ds == today.Format("2006-01-02") {
			break
		}
		base := 2.5 + float64(doneCount)*0.35 + rng.NormFloat64()*0.6
		for _, t := range trackers {
			switch t.Kind {
			case "scale":
				v := math.Round(clamp(base+rng.NormFloat64()*0.5, t.MinValue, t.MaxValue))
				_ = s.store.SetTrackerEntry(ctx, t.ID, ds, &v)
			case "yesno":
				v := 0.0
				if base < 3 && rng.Float64() < 0.6 || rng.Float64() < 0.12 {
					v = 1
				}
				_ = s.store.SetTrackerEntry(ctx, t.ID, ds, &v)
			default:
				if rng.Float64() < 0.8 {
					weight += rng.NormFloat64()*0.25 - 0.03
					v := math.Round(weight*10) / 10
					_ = s.store.SetTrackerEntry(ctx, t.ID, ds, &v)
				}
			}
		}
		if rng.Float64() < 0.85 {
			_ = s.store.SetJournal(ctx, ds, lines[rng.Intn(len(lines))], nil)
		}
		// Now and then a day worth remembering, marked red on the month page.
		if rng.Float64() < 0.06 {
			_ = s.store.SetJournalImportant(ctx, ds, important[rng.Intn(len(important))])
		}
	}
	s.log.Info("demo data seeded")
	return nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
