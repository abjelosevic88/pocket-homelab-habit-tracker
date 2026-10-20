package store

import (
	"context"
	"database/sql"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

type entrySQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ownsEntry(ctx context.Context, db entrySQL, table string, uid, id int64) bool {
	var one int
	return db.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id=? AND user_id=?`, id, uid).Scan(&one) == nil
}

// SaveCheckin commits the entire day and its publication together.
func (s *Store) SaveCheckin(ctx context.Context, uid int64, date string, habits map[int64]int, trackers map[int64]*float64, text, bad, important *string, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, v := range habits {
		if err := setHabitEntry(ctx, tx, uid, id, date, v); err != nil {
			return err
		}
	}
	for id, v := range trackers {
		if err := setTrackerEntry(ctx, tx, uid, id, date, v); err != nil {
			return err
		}
	}
	if text != nil || bad != nil || important != nil {
		if err := setJournalFields(ctx, tx, uid, date, text, bad, important); err != nil {
			return err
		}
	}
	return core.Commit(tx, after...)
}
