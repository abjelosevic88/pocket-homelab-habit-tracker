package store

import "context"

// JournalEntry is one day's lines.
type JournalEntry struct {
	Text string `json:"text"`
	Bad  string `json:"bad"`
	// Important is something to remember about the day; the month page marks the day red.
	Important string `json:"important"`
}

// SetJournalFields writes the fields that are not nil and leaves the others as they are.
// A day with every field empty is deleted.
func (s *Store) SetJournalFields(ctx context.Context, date string, text, bad, important *string) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	cur := s.journalDay(ctx, date)
	if text != nil {
		cur.Text = *text
	}
	if bad != nil {
		cur.Bad = *bad
	}
	if important != nil {
		cur.Important = *important
	}
	if cur.Text == "" && cur.Bad == "" && cur.Important == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM journal WHERE date = ?`, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO journal (date, text, bad_text, important, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET text = excluded.text, bad_text = excluded.bad_text, important = excluded.important, updated_at = excluded.updated_at`,
		date, cur.Text, cur.Bad, cur.Important, Now())
	return err
}

// SetJournal writes the day's good line; a nil bad leaves the bad line untouched.
func (s *Store) SetJournal(ctx context.Context, date string, text string, bad *string) error {
	return s.SetJournalFields(ctx, date, &text, bad, nil)
}

// SetJournalBad writes only the bad line.
func (s *Store) SetJournalBad(ctx context.Context, date, bad string) error {
	return s.SetJournalFields(ctx, date, nil, &bad, nil)
}

// SetJournalImportant writes only the day's important note.
func (s *Store) SetJournalImportant(ctx context.Context, date, important string) error {
	return s.SetJournalFields(ctx, date, nil, nil, &important)
}

func (s *Store) journalDay(ctx context.Context, date string) JournalEntry {
	var e JournalEntry
	_ = s.DB.QueryRowContext(ctx, `SELECT text, bad_text, important FROM journal WHERE date = ?`, date).Scan(&e.Text, &e.Bad, &e.Important)
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

// JournalFull returns every field by date for a range.
func (s *Store) JournalFull(ctx context.Context, from, to string) (map[string]JournalEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, text, bad_text, important FROM journal WHERE date BETWEEN ? AND ? ORDER BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]JournalEntry{}
	for rows.Next() {
		var d string
		var e JournalEntry
		if err := rows.Scan(&d, &e.Text, &e.Bad, &e.Important); err != nil {
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
