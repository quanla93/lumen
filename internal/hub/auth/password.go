// Package auth owns password hashing (Argon2id), JWT issue/verify, the
// session cookie shape, and the RequireSession middleware. It also holds
// the small users table CRUD since auth and user storage are tightly
// coupled.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters tuned for a homelab hub (RFC 9106 second-class
// memory profile — 19 MB working set, fast enough on a Pi 4).
const (
	argonMemoryKB  = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonSaltBytes = 16
	argonKeyBytes  = 32
)

// HashPassword returns an encoded Argon2id hash of the form
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt-b64>$<hash-b64>
//
// which is self-describing — VerifyPassword can re-derive without needing
// the parameters out-of-band.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKB, argonThreads, argonKeyBytes)
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

// VerifyPassword checks password against an encoded hash from HashPassword.
// Returns true on match; false on any mismatch or parse error. Uses
// constant-time comparison.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "salt", "hash"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ErrInvalidCredentials is returned by the login handler when username
// or password doesn't match. Kept opaque on purpose.
var ErrInvalidCredentials = errors.New("invalid credentials")
