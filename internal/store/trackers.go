package store

import (
	"context"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"strings"
)

const trackerCols = `id, name, kind, min_value, max_value, step, unit, higher_is_better, sort_order, start_date, end_date, default_value, created_at`

func scanTracker(row interface{ Scan(...any) error }) (*Tracker, error) {
	var t Tracker
	var hib int
	if err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.MinValue, &t.MaxValue, &t.Step, &t.Unit, &hib, &t.SortOrder, &t.StartDate, &t.EndDate, &t.DefaultValue, &t.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	t.HigherIsBetter = hib == 1
	return &t, nil
}

// ListTrackers mirrors ListHabits for trackers.
func (s *Store) ListTrackers(ctx context.Context, uid int64, month string, includeEnded bool) ([]Tracker, error) {
	q := `SELECT ` + trackerCols + ` FROM trackers`
	args := []any{uid}
	where := []string{`user_id = ?`}
	if month != "" {
		first, last, err := MonthBounds(month)
		if err != nil {
			return nil, err
		}
		where = append(where, `start_date <= ? AND (end_date IS NULL OR end_date >= ?)`)
		args = append(args, last.Format("2006-01-02"), first.Format("2006-01-02"))
	} else if !includeEnded {
		where = append(where, `(end_date IS NULL OR end_date >= ?)`)
		args = append(args, Today())
	}
	q += ` WHERE ` + strings.Join(where, " AND ")
	q += ` ORDER BY (end_date IS NOT NULL), sort_order, id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tracker{}
	for rows.Next() {
		t, err := scanTracker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTracker loads one of the user's trackers; another user's tracker is ErrNotFound.
func (s *Store) GetTracker(ctx context.Context, uid, id int64) (*Tracker, error) {
	return scanTracker(s.DB.QueryRowContext(ctx, `SELECT `+trackerCols+` FROM trackers WHERE id = ? AND user_id = ?`, id, uid))
}

// CreateTracker inserts a tracker at the end of the list.
func (s *Store) CreateTracker(ctx context.Context, uid int64, t *Tracker) error {
	if t.StartDate == "" {
		t.StartDate = Today()
	}
	if t.Step <= 0 {
		t.Step = 1
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM trackers WHERE user_id = ?`, uid).Scan(&max)
	t.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO trackers (user_id, name, kind, min_value, max_value, step, unit, higher_is_better, sort_order, start_date, end_date, default_value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uid, t.Name, t.Kind, t.MinValue, t.MaxValue, t.Step, t.Unit, boolInt(t.HigherIsBetter), t.SortOrder, t.StartDate, t.EndDate, t.DefaultValue)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.CreatedAt = Now()
	return nil
}

// UpdateTracker saves one of the user's trackers.
func (s *Store) UpdateTracker(ctx context.Context, uid int64, t *Tracker) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE trackers SET name = ?, kind = ?, min_value = ?, max_value = ?, step = ?, unit = ?, higher_is_better = ?, sort_order = ?, start_date = ?, end_date = ?, default_value = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		t.Name, t.Kind, t.MinValue, t.MaxValue, t.Step, t.Unit, boolInt(t.HigherIsBetter), t.SortOrder, t.StartDate, t.EndDate, t.DefaultValue, Now(), t.ID, uid)
	return affected(res, err)
}

// DeleteTracker removes one of the user's trackers and its entries.
func (s *Store) DeleteTracker(ctx context.Context, uid, id int64) error {
	return affected(s.DB.ExecContext(ctx, `DELETE FROM trackers WHERE id = ? AND user_id = ?`, id, uid))
}

// ReorderTrackers applies the given id order; ids that are not the user's are ignored.
func (s *Store) ReorderTrackers(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE trackers SET sort_order = ? WHERE id = ? AND user_id = ?`, i+1, id, uid); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SetTrackerEntry writes a value of one of the user's trackers; a nil value clears the day.
// Another user's tracker is ErrNotFound.
func (s *Store) SetTrackerEntry(ctx context.Context, uid, trackerID int64, date string, value *float64, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setTrackerEntry(ctx, tx, uid, trackerID, date, value); err != nil {
		return err
	}
	return core.Commit(tx, after...)
}
func setTrackerEntry(ctx context.Context, db entrySQL, uid, trackerID int64, date string, value *float64) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	if !ownsEntry(ctx, db, "trackers", uid, trackerID) {
		return ErrNotFound
	}
	if value == nil {
		_, err := db.ExecContext(ctx, `DELETE FROM tracker_entries WHERE tracker_id = ? AND date = ? AND user_id = ?`, trackerID, date, uid)
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO tracker_entries (tracker_id, date, value, updated_at, user_id) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(tracker_id, date) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, trackerID, date, *value, Now(), uid)
	return err
}

// FillDefaults adds each tracker's default value to every active day in
// [from, to] up to today that has no entry, and reports which days it filled.
// Future days are left empty: a default is "what happened unless you said
// otherwise", not a prediction.
func FillDefaults(trackers []Tracker, entries map[int64]map[string]float64, from, to, today string) map[int64]map[string]bool {
	filled := map[int64]map[string]bool{}
	start, err := ParseDate(from)
	if err != nil {
		return filled
	}
	end, err := ParseDate(to)
	if err != nil {
		return filled
	}
	if to > today {
		end, _ = ParseDate(today)
	}
	for _, t := range trackers {
		if t.DefaultValue == nil {
			continue
		}
		if entries[t.ID] == nil {
			entries[t.ID] = map[string]float64{}
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			if !t.ActiveOn(ds) {
				continue
			}
			if _, ok := entries[t.ID][ds]; ok {
				continue
			}
			entries[t.ID][ds] = *t.DefaultValue
			if filled[t.ID] == nil {
				filled[t.ID] = map[string]bool{}
			}
			filled[t.ID][ds] = true
		}
	}
	return filled
}

// TrackerEntries returns the user's values by tracker id and date for a date range.
func (s *Store) TrackerEntries(ctx context.Context, uid int64, from, to string) (map[int64]map[string]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tracker_id, date, value FROM tracker_entries WHERE user_id = ? AND date BETWEEN ? AND ?`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]float64{}
	for rows.Next() {
		var id int64
		var d string
		var v float64
		if err := rows.Scan(&id, &d, &v); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]float64{}
		}
		out[id][d] = v
	}
	return out, rows.Err()
}

// ActiveOn reports whether the tracker's date bounds include the date.
func (t Tracker) ActiveOn(date string) bool {
	if date < t.StartDate {
		return false
	}
	if t.EndDate != nil && date > *t.EndDate {
		return false
	}
	return true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
