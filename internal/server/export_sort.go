package server

import "sort"

func sortRows(ex *Export) {
	sort.Slice(ex.HabitEntries, func(i, j int) bool {
		a, b := ex.HabitEntries[i], ex.HabitEntries[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.HabitID < b.HabitID
	})
	sort.Slice(ex.TrackerEntries, func(i, j int) bool {
		a, b := ex.TrackerEntries[i], ex.TrackerEntries[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.TrackerID < b.TrackerID
	})
	sort.Slice(ex.Journal, func(i, j int) bool { return ex.Journal[i].Date < ex.Journal[j].Date })
}
