package store

import "context"

// JournalEntry is one day's lines.
type JournalEntry struct {
	Text string `json:"text"`
	Bad  string `json:"bad"`
}

// SetJournal writes the day's good line; a nil bad leaves the bad line untouched.
// A day with both lines empty is deleted.
func (s *Store) SetJournal(ctx context.Context, date string, text string, bad *string) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	cur := s.journalDay(ctx, date)
	cur.Text = text
	if bad != nil {
		cur.Bad = *bad
	}
	if cur.Text == "" && cur.Bad == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM journal WHERE date = ?`, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO journal (date, text, bad_text, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET text = excluded.text, bad_text = excluded.bad_text, updated_at = excluded.updated_at`, date, cur.Text, cur.Bad, Now())
	return err
}

// SetJournalBad writes only the bad line.
func (s *Store) SetJournalBad(ctx context.Context, date, bad string) error {
	cur := s.journalDay(ctx, date)
	return s.SetJournal(ctx, date, cur.Text, &bad)
}

func (s *Store) journalDay(ctx context.Context, date string) JournalEntry {
	var e JournalEntry
	_ = s.DB.QueryRowContext(ctx, `SELECT text, bad_text FROM journal WHERE date = ?`, date).Scan(&e.Text, &e.Bad)
	return e
}

// Journal returns good lines by date for a range.
func (s *Store) Journal(ctx context.Context, from, to string) (map[string]string, error) {
	full, err := s.JournalFull(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for d, e := range full {
		if e.Text != "" {
			out[d] = e.Text
		}
	}
	return out, nil
}

// JournalFull returns both lines by date for a range.
func (s *Store) JournalFull(ctx context.Context, from, to string) (map[string]JournalEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, text, bad_text FROM journal WHERE date BETWEEN ? AND ? ORDER BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]JournalEntry{}
	for rows.Next() {
		var d string
		var e JournalEntry
		if err := rows.Scan(&d, &e.Text, &e.Bad); err != nil {
			return nil, err
		}
		out[d] = e
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
