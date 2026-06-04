package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct {
	ID        int64
	Username  string
	CreatedAt time.Time
}

// HasAny returns true if at least one user exists. Used by the
// /api/setup-status endpoint to decide whether to show register or
// login on first paint.
func HasAny(ctx context.Context, db *sql.DB) (bool, error) {
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateUser inserts a new user with the given pre-hashed password.
// Returns the new user. Caller must hash the password before calling.
func CreateUser(ctx context.Context, db *sql.DB, username, passwordHash string) (User, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		username, passwordHash,
	)
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return GetUserByID(ctx, db, id)
}

// GetUserByID returns the user with that id, or sql.ErrNoRows if absent.
func GetUserByID(ctx context.Context, db *sql.DB, id int64) (User, error) {
	var u User
	err := db.QueryRowContext(ctx,
		`SELECT id, username, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// GetUserByUsername is used by Login. Returns ErrInvalidCredentials if no
// such user — same error shape as a bad password, so timing/error leakage
// can't distinguish "user doesn't exist" from "wrong password".
func GetUserByUsername(ctx context.Context, db *sql.DB, username string) (User, string, error) {
	var u User
	var hash string
	err := db.QueryRowContext(ctx,
		`SELECT id, username, created_at, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	return u, hash, nil
}

// getUserByIDWithHash is the same as GetUserByID but also returns the
// current password hash — used by ChangePassword to verify the
// caller-supplied current password before rewriting it.
func getUserByIDWithHash(ctx context.Context, db *sql.DB, id int64) (User, string, error) {
	var u User
	var hash string
	err := db.QueryRowContext(ctx,
		`SELECT id, username, created_at, password_hash FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt, &hash)
	return u, hash, err
}

// GetSingleAdmin returns the single user that backs Lumen's
// "single-admin" model. Used by the OIDC callback to find the local
// user to bind the new session to (any OIDC login attaches to this one
// row). Returns sql.ErrNoRows if no user is registered yet so the caller
// can redirect to /register.
func GetSingleAdmin(ctx context.Context, db *sql.DB) (User, error) {
	var u User
	err := db.QueryRowContext(ctx,
		`SELECT id, username, created_at FROM users ORDER BY id ASC LIMIT 1`,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// UpdatePasswordHash rewrites the password_hash column. Caller is
// responsible for hashing (and for verifying the current password
// first if this is a self-service change).
func UpdatePasswordHash(ctx context.Context, db *sql.DB, id int64, hash string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id,
	)
	if err != nil {
		return fmt.Errorf("update password_hash: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
