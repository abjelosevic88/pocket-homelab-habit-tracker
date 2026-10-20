package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// CountUsers returns the number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a user. The very first account also claims anything that was made
// before anyone existed (an old box upgraded with no account yet, auth mode "none"/"proxy").
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users (email, name, password_hash, role) VALUES (?, ?, ?, ?)`, u.Email, u.Name, u.PasswordHash, u.Role)
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	if n, err := s.CountUsers(ctx); err == nil && n == 1 {
		return s.claimOrphans(ctx, u.ID)
	}
	return nil
}

// contentTables carry a user_id; every row belongs to exactly one person.
var contentTables = []string{"habits", "trackers", "habit_entries", "tracker_entries", "journal", "weather_days", "location_days"}

// claimOrphans gives rows without an owner, and personal settings still stored box-wide, to uid.
func (s *Store) claimOrphans(ctx context.Context, uid int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range contentTables {
		if _, err := tx.ExecContext(ctx, `UPDATE `+t+` SET user_id = ? WHERE user_id IS NULL`, uid); err != nil {
			return err
		}
	}
	for _, k := range personalKeys {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_settings (user_id, key, value) SELECT ?, key, value FROM settings WHERE key = ?`, uid, k); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsAdmin reports whether the account may change box-wide settings.
func (u *User) IsAdmin() bool { return u != nil && u.Role == "admin" }

// UpdateUser updates name/email/password/role.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET email = ?, name = ?, password_hash = ?, role = ?, updated_at = ? WHERE id = ?`, u.Email, u.Name, u.PasswordHash, u.Role, Now(), u.ID)
	return err
}

const userCols = `id, email, name, password_hash, role, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

// GetUserByEmail finds a user by email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, email))
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// RandomToken returns n random bytes hex encoded.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HashToken returns the sha256 hex digest of a token.
func HashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// CreateSession creates a session for a user and returns its id.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	id := RandomToken(32)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, HashToken(id), userID, time.Now().Add(ttl).UTC().Format(time.RFC3339))
	return id, err
}

// GetSessionUser resolves a session id to a user.
func (s *Store) GetSessionUser(ctx context.Context, sessionID string) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, u.password_hash, u.role, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = ? AND s.expires_at > ?`, HashToken(sessionID), time.Now().UTC().Format(time.RFC3339)))
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, HashToken(sessionID))
	return err
}

// PurgeSessions removes expired sessions.
func (s *Store) PurgeSessions(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// CreateAPIToken creates a token and returns the plaintext (shown once).
func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name string) (string, *APIToken, error) {
	plain := "ph_" + RandomToken(24)
	prefix := plain[:10]
	res, err := s.DB.ExecContext(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, prefix) VALUES (?, ?, ?, ?)`, userID, name, HashToken(plain), prefix)
	if err != nil {
		return "", nil, err
	}
	id, _ := res.LastInsertId()
	return plain, &APIToken{ID: id, UserID: userID, Name: name, Prefix: prefix, CreatedAt: Now()}, nil
}

// GetUserByAPIToken resolves a bearer token.
func (s *Store) GetUserByAPIToken(ctx context.Context, token string) (*User, error) {
	h := HashToken(token)
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, u.password_hash, u.role, u.created_at
		FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, h))
	if err != nil {
		return nil, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?`, Now(), h)
	return u, nil
}

// ListAPITokens lists tokens for a user.
func (s *Store) ListAPITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, prefix, last_used_at, created_at FROM api_tokens WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteAPIToken removes a token.
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=?`, id))
}
