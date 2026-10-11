package store

import "context"

// SetJournal writes the day's line; empty text deletes it.
func (s *Store) SetJournal(ctx context.Context, date, text string) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	if text == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM journal WHERE date = ?`, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO journal (date, text, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET text = excluded.text, updated_at = excluded.updated_at`, date, text, Now())
	return err
}

// Journal returns entries by date for a range.
func (s *Store) Journal(ctx context.Context, from, to string) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, text FROM journal WHERE date BETWEEN ? AND ? ORDER BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, t string
		if err := rows.Scan(&d, &t); err != nil {
			return nil, err
		}
		out[d] = t
	}
	return out, rows.Err()
}

// Counts returns table sizes for metrics / system info.
func (s *Store) Counts(ctx context.Context) map[string]int {
	out := map[string]int{}
	for _, t := range []string{"habits", "habit_entries", "trackers", "tracker_entries", "journal", "users"} {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+t).Scan(&n)
		out[t] = n
	}
	return out
}
