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
func (s *Store) SetJournalFields(ctx context.Context, uid int64, date string, text, bad, important *string) error {
	if _, err := ParseDate(date); err != nil {
		return err
	}
	cur := s.journalDay(ctx, uid, date)
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
		_, err := s.DB.ExecContext(ctx, `DELETE FROM journal WHERE user_id = ? AND date = ?`, uid, date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO journal (user_id, date, text, bad_text, important, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET text = excluded.text, bad_text = excluded.bad_text, important = excluded.important, updated_at = excluded.updated_at`,
		uid, date, cur.Text, cur.Bad, cur.Important, Now())
	return err
}

// SetJournal writes the day's good line; a nil bad leaves the bad line untouched.
func (s *Store) SetJournal(ctx context.Context, uid int64, date string, text string, bad *string) error {
	return s.SetJournalFields(ctx, uid, date, &text, bad, nil)
}

// SetJournalBad writes only the bad line.
func (s *Store) SetJournalBad(ctx context.Context, uid int64, date, bad string) error {
	return s.SetJournalFields(ctx, uid, date, nil, &bad, nil)
}

// SetJournalImportant writes only the day's important note.
func (s *Store) SetJournalImportant(ctx context.Context, uid int64, date, important string) error {
	return s.SetJournalFields(ctx, uid, date, nil, nil, &important)
}

func (s *Store) journalDay(ctx context.Context, uid int64, date string) JournalEntry {
	var e JournalEntry
	_ = s.DB.QueryRowContext(ctx, `SELECT text, bad_text, important FROM journal WHERE user_id = ? AND date = ?`, uid, date).Scan(&e.Text, &e.Bad, &e.Important)
	return e
}

// Journal returns the user's good lines by date for a range.
func (s *Store) Journal(ctx context.Context, uid int64, from, to string) (map[string]string, error) {
	full, err := s.JournalFull(ctx, uid, from, to)
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

// JournalFull returns every field of the user's journal by date for a range.
func (s *Store) JournalFull(ctx context.Context, uid int64, from, to string) (map[string]JournalEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, text, bad_text, important FROM journal WHERE user_id = ? AND date BETWEEN ? AND ? ORDER BY date`, uid, from, to)
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

// Counts returns table sizes: the user's own rows, or the whole box's for uid 0 (metrics).
func (s *Store) Counts(ctx context.Context, uid int64) map[string]int {
	out := map[string]int{}
	for _, t := range []string{"habits", "habit_entries", "trackers", "tracker_entries", "journal"} {
		var n int
		if uid == 0 {
			_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+t).Scan(&n)
		} else {
			_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+t+` WHERE user_id = ?`, uid).Scan(&n)
		}
		out[t] = n
	}
	if uid == 0 {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
		out["users"] = n
	}
	return out
}
