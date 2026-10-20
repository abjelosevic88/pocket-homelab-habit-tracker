package server

import (
	"context"
	"database/sql"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-habit-tracker/internal/store"
)

func (s *Server) userEvent(ctx context.Context, u *store.User, topic, subject string, payload map[string]any) core.CommitHook {
	if !s.core.Configured() || u == nil {
		return nil
	}
	payload["user"] = u.Email
	return func(tx *sql.Tx) error { return core.QueueEvent(ctx, tx, topic, subject, payload) }
}
