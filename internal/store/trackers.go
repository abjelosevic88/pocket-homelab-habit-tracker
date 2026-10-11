package store

import (
	"context"
	"strings"
)

const trackerCols = `id, name, kind, min_value, max_value, step, unit, higher_is_better, sort_order, start_date, end_date, created_at`

func scanTracker(row interface{ Scan(...any) error }) (*Tracker, error) {
	var t Tracker
	var hib int
	if err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.MinValue, &t.MaxValue, &t.Step, &t.Unit, &hib, &t.SortOrder, &t.StartDate, &t.EndDate, &t.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	t.HigherIsBetter = hib == 1
	return &t, nil
}

// ListTrackers mirrors ListHabits for trackers.
func (s *Store) ListTrackers(ctx context.Context, month string, includeEnded bool) ([]Tracker, error) {
	q := `SELECT ` + trackerCols + ` FROM trackers`
	var args []any
	var where []string
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
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
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

// GetTracker loads one tracker.
func (s *Store) GetTracker(ctx context.Context, id int64) (*Tracker, error) {
	return scanTracker(s.DB.QueryRowContext(ctx, `SELECT `+trackerCols+` FROM trackers WHERE id = ?`, id))
}

// CreateTracker inserts a tracker at the end of the list.
func (s *Store) CreateTracker(ctx context.Context, t *Tracker) error {
	if t.StartDate == "" {
		t.StartDate = Today()
	}
	if t.Step <= 0 {
		t.Step = 1
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM trackers`).Scan(&max)
	t.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO trackers (name, kind, min_value, max_value, step, unit, higher_is_better, sort_order, start_date, end_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, t.Name, t.Kind, t.MinValue, t.MaxValue, t.Step, t.Unit, boolInt(t.HigherIsBetter), t.SortOrder, t.StartDate, t.EndDate)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.CreatedAt = Now()
	return nil
}

// UpdateTracker saves a tracker.
func (s *Store) UpdateTracker(ctx context.Context, t *Tracker) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE trackers SET name = ?, kind = ?, min_value = ?, max_value = ?, step = ?, unit = ?, higher_is_better = ?, sort_order = ?, start_date = ?, end_date = ?, updated_at = ? WHERE id = ?`,
		t.Name, t.Kind, t.MinValue, t.MaxValue, t.Step, t.Unit, boolInt(t.HigherIsBetter), t.SortOrder, t.StartDate, t.EndDate, Now(), t.ID)
	return err
}

// DeleteTracker removes a tracker and its entries.
func (s *Store) DeleteTracker(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM trackers WHERE id = ?`, id)
	return err
}

// ReorderTrackers applies the given id order.
func (s *Store) ReorderTrackers(ctx context.Context, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE trackers SET sort_order = ? WHERE id = ?`, i+1, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SetTrackerEntry writes a value; a nil value clears the day.
func (s *Store) SetTrackerEntry(ctx context.Context, trackerID int64, date string, value *float64) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	if value == nil {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM tracker_entries WHERE tracker_id = ? AND date = ?`, trackerID, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO tracker_entries (tracker_id, date, value, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(tracker_id, date) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, trackerID, date, *value, Now())
	return err
}

// TrackerEntries returns values by tracker id and date for a date range.
func (s *Store) TrackerEntries(ctx context.Context, from, to string) (map[int64]map[string]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tracker_id, date, value FROM tracker_entries WHERE date BETWEEN ? AND ?`, from, to)
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
