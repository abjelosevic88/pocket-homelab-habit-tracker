package store

import (
	"context"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
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
func (s *Store) ListHabits(ctx context.Context, uid int64, month string, includeEnded bool) ([]Habit, error) {
	q := `SELECT ` + habitCols + ` FROM habits`
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

// GetHabit loads one of the user's habits; another user's habit is ErrNotFound.
func (s *Store) GetHabit(ctx context.Context, uid, id int64) (*Habit, error) {
	return scanHabit(s.DB.QueryRowContext(ctx, `SELECT `+habitCols+` FROM habits WHERE id = ? AND user_id = ?`, id, uid))
}

// CreateHabit inserts a habit at the end of the list.
func (s *Store) CreateHabit(ctx context.Context, uid int64, h *Habit) error {
	if h.StartDate == "" {
		h.StartDate = Today()
	}
	if h.Days == "" {
		h.Days = "1234567"
	}
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM habits WHERE user_id = ?`, uid).Scan(&max)
	h.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO habits (user_id, name, color, days, sort_order, start_date, end_date) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uid, h.Name, h.Color, h.Days, h.SortOrder, h.StartDate, h.EndDate)
	if err != nil {
		return err
	}
	h.ID, _ = res.LastInsertId()
	h.CreatedAt = Now()
	return nil
}

// UpdateHabit saves name, colour, schedule and date bounds of one of the user's habits.
func (s *Store) UpdateHabit(ctx context.Context, uid int64, h *Habit) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE habits SET name = ?, color = ?, days = ?, sort_order = ?, start_date = ?, end_date = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		h.Name, h.Color, h.Days, h.SortOrder, h.StartDate, h.EndDate, Now(), h.ID, uid)
	return affected(res, err)
}

// DeleteHabit removes one of the user's habits and all its entries.
func (s *Store) DeleteHabit(ctx context.Context, uid, id int64) error {
	return affected(s.DB.ExecContext(ctx, `DELETE FROM habits WHERE id = ? AND user_id = ?`, id, uid))
}

// ReorderHabits applies the given id order; ids that are not the user's are ignored.
func (s *Store) ReorderHabits(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE habits SET sort_order = ? WHERE id = ? AND user_id = ?`, i+1, id, uid); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SetHabitEntry sets (status 1/-1) or clears (status 0) a day of one of the user's habits;
// another user's habit is ErrNotFound.
func (s *Store) SetHabitEntry(ctx context.Context, uid, habitID int64, date string, status int, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setHabitEntry(ctx, tx, uid, habitID, date, status); err != nil {
		return err
	}
	return core.Commit(tx, after...)
}
func setHabitEntry(ctx context.Context, db entrySQL, uid, habitID int64, date string, status int) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	if !ownsEntry(ctx, db, "habits", uid, habitID) {
		return ErrNotFound
	}
	if status == 0 {
		_, err := db.ExecContext(ctx, `DELETE FROM habit_entries WHERE habit_id = ? AND date = ? AND user_id = ?`, habitID, date, uid)
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO habit_entries (habit_id, date, status, updated_at, user_id) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(habit_id, date) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at`, habitID, date, status, Now(), uid)
	return err
}

// HabitEntries returns the user's status by habit id and date for a date range (inclusive).
func (s *Store) HabitEntries(ctx context.Context, uid int64, from, to string) (map[int64]map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT habit_id, date, status FROM habit_entries WHERE user_id = ? AND date BETWEEN ? AND ?`, uid, from, to)
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
