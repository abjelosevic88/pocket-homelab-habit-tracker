package store

import (
	"context"
	"strings"
	"time"
)

const habitCols = `id, name, color, days, sort_order, start_date, end_date, created_at`

func scanHabit(row interface{ Scan(...any) error }) (*Habit, error) {
	var h Habit
	if err := row.Scan(&h.ID, &h.Name, &h.Color, &h.Days, &h.SortOrder, &h.StartDate, &h.EndDate, &h.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &h, nil
}

// ListHabits returns every habit, active ones first. When month (YYYY-MM) is
// given only habits that overlap that month are returned.
func (s *Store) ListHabits(ctx context.Context, month string, includeEnded bool) ([]Habit, error) {
	q := `SELECT ` + habitCols + ` FROM habits`
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
	out := []Habit{}
	for rows.Next() {
		h, err := scanHabit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

// GetHabit loads one habit.
func (s *Store) GetHabit(ctx context.Context, id int64) (*Habit, error) {
	return scanHabit(s.DB.QueryRowContext(ctx, `SELECT `+habitCols+` FROM habits WHERE id = ?`, id))
}

// CreateHabit inserts a habit at the end of the list.
func (s *Store) CreateHabit(ctx context.Context, h *Habit) error {
	if h.StartDate == "" {
		h.StartDate = Today()
	}
	if h.Days == "" {
		h.Days = "1234567"
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM habits`).Scan(&max)
	h.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO habits (name, color, days, sort_order, start_date, end_date) VALUES (?, ?, ?, ?, ?, ?)`,
		h.Name, h.Color, h.Days, h.SortOrder, h.StartDate, h.EndDate)
	if err != nil {
		return err
	}
	h.ID, _ = res.LastInsertId()
	h.CreatedAt = Now()
	return nil
}

// UpdateHabit saves name, colour, schedule and date bounds.
func (s *Store) UpdateHabit(ctx context.Context, h *Habit) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE habits SET name = ?, color = ?, days = ?, sort_order = ?, start_date = ?, end_date = ?, updated_at = ? WHERE id = ?`,
		h.Name, h.Color, h.Days, h.SortOrder, h.StartDate, h.EndDate, Now(), h.ID)
	return err
}

// DeleteHabit removes a habit and all its entries.
func (s *Store) DeleteHabit(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM habits WHERE id = ?`, id)
	return err
}

// ReorderHabits applies the given id order.
func (s *Store) ReorderHabits(ctx context.Context, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE habits SET sort_order = ? WHERE id = ?`, i+1, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SetHabitEntry sets (status 1/-1) or clears (status 0) a day.
func (s *Store) SetHabitEntry(ctx context.Context, habitID int64, date string, status int) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	if status == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM habit_entries WHERE habit_id = ? AND date = ?`, habitID, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO habit_entries (habit_id, date, status, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(habit_id, date) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`, habitID, date, status, Now())
	return err
}

// HabitEntries returns status by habit id and date for a date range (inclusive).
func (s *Store) HabitEntries(ctx context.Context, from, to string) (map[int64]map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT habit_id, date, status FROM habit_entries WHERE date BETWEEN ? AND ?`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]int{}
	for rows.Next() {
		var id int64
		var d string
		var st int
		if err := rows.Scan(&id, &d, &st); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]int{}
		}
		out[id][d] = st
	}
	return out, rows.Err()
}

// Scheduled reports whether the habit is scheduled on the given date.
func (h Habit) Scheduled(d time.Time) bool {
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7
	}
	return strings.ContainsRune(h.Days, rune('0'+wd))
}

// ActiveOn reports whether the habit's date bounds include the date.
func (h Habit) ActiveOn(date string) bool {
	if date < h.StartDate {
		return false
	}
	if h.EndDate != nil && date > *h.EndDate {
		return false
	}
	return true
}
