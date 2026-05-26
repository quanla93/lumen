package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// EnsureUser is a startup helper: if username is non-empty AND the user
// doesn't already exist, create it with the given plaintext password
// (Argon2id-hashed at write time). If the user already exists, no-op —
// password changes made through the UI are NOT overwritten on restart.
//
// Both arguments empty → no-op, returns nil (env-seed disabled).
// Only one set → returns an error so misconfig is loud.
//
// The point: an operator sets LUMEN_HUB_ADMIN_USERNAME +
// LUMEN_HUB_ADMIN_PASSWORD in .env once, and the admin is there on every
// fresh deploy — no first-admin dance after dropping the SQLite volume.
func EnsureUser(ctx context.Context, db *sql.DB, username, password string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("LUMEN_HUB_ADMIN_USERNAME and LUMEN_HUB_ADMIN_PASSWORD must be set together (or both empty)")
	}

	_, _, err := GetUserByUsername(ctx, db, username)
	switch {
	case err == nil:
		logger.Info("seed admin already exists, leaving password untouched", "user", username)
		return nil
	case errors.Is(err, ErrInvalidCredentials):
		// fall through — user doesn't exist, create it
	default:
		return fmt.Errorf("lookup user: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u, err := CreateUser(ctx, db, username, hash)
	if err != nil {
		return fmt.Errorf("create seed user: %w", err)
	}
	logger.Info("seed admin created", "user", u.Username, "id", u.ID)
	return nil
}
