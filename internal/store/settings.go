package store

import (
	"context"
	"strconv"
)

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: "Pocket Habits", WeekStart: 1, Landing: "month", JournalPrompt: "What was great about today?", ReminderFormat: "ntfy"}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "week_start":
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 7 {
				st.WeekStart = n
			}
		case "landing":
			if v == "today" || v == "month" {
				st.Landing = v
			}
		case "journal_prompt":
			if v != "" {
				st.JournalPrompt = v
			}
		case "reminder_time":
			st.ReminderTime = v
		case "reminder_url":
			st.ReminderURL = v
		case "reminder_format":
			if v != "" {
				st.ReminderFormat = v
			}
		case "reminder_last":
			st.ReminderLast = v
		case "setup_complete":
			st.SetupComplete = v == "1"
		case "defaults_seeded":
			st.DefaultsSeeded = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	kv := map[string]string{
		"title":           st.Title,
		"week_start":      strconv.Itoa(st.WeekStart),
		"landing":         st.Landing,
		"journal_prompt":  st.JournalPrompt,
		"reminder_time":   st.ReminderTime,
		"reminder_url":    st.ReminderURL,
		"reminder_format": st.ReminderFormat,
		"reminder_last":   st.ReminderLast,
		"setup_complete":  b(st.SetupComplete),
		"defaults_seeded": b(st.DefaultsSeeded),
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SetSetting writes a single key.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
