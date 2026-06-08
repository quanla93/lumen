// saml_userlookup.go — bridge the validated SAML NameID to the
// existing single-admin row in the users table.
//
// The single-admin gate is enforced at ACS (SAML flow rejects
// NameIDs that aren't in the expected_nameid list). At that point
// we're guaranteed the NameID is allowed; this helper just looks up
// the matching user row. Lumen's single-admin model means there's
// exactly one row to find (or zero — a misconfiguration).
//
// Multi-user (Sprint 9 / RFC 0008) will change this to honour SAML
// attributes (group/role mapping). For v1 we bind to the existing
// password-admin row so an operator can either use passwords or
// SAML — same single session cookie either way.

package auth

import (
	"context"
	"database/sql"
	"errors"
)

// lookupUserIDByUsername returns the user.id for the given username,
// or 0 if the row doesn't exist. Errors are returned for any other
// failure (db unavailable, etc).
//
// 0 is a sentinel — the users table is auto-increment starting at
// 1, so an id of 0 is never a real value. The handler treats 0 as
// "no matching user" and bounces the caller back to /login.
func lookupUserIDByUsername(ctx context.Context, db *sql.DB, username string) (int64, error) {
	if username == "" {
		return 0, errors.New("saml: empty username")
	}
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE username = ?`, username,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}
