// passphrase_hash.go — Argon2id hash for the operator's passphrase.
//
// Stored at `backup.passphrase_hash` so the CLI restore path can
// surface "wrong passphrase" without keeping the passphrase itself.
// The cost matches DefaultArgon2Params (the same cost Seal/Open
// use) so verification is the same wall time as the actual decrypt —
// no surprise to the operator that "verify" is faster than "decrypt".
//
// We use the standard argon2 encoded form so a future CLI written
// in any language can verify against the same hash string without
// the Lumen binary on hand.

package backup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// hashWithArgon2 derives a 32-byte key with the same params Seal
// uses, and returns the canonical encoded form:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<saltB64>$<hashB64>
func hashWithArgon2(passphrase string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	p := DefaultArgon2Params
	hash := argon2.IDKey([]byte(passphrase), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// verifyPassphrase checks typed against the stored hash. Returns
// nil on match, errors.New on mismatch or malformed input. Constant
// time on the bytes; not constant time on the parse path (the
// encoded form is the only public way to verify, so an attacker
// would have to break Argon2id to exploit the parse).
func verifyPassphrase(typed, stored string) error {
	if stored == "" {
		return errors.New("backup: no passphrase set (use --set-passphrase or the Settings UI)")
	}
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("backup: malformed stored hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return errors.New("backup: malformed version")
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return errors.New("backup: malformed params")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return fmt.Errorf("backup: decode salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return fmt.Errorf("backup: decode hash: %w", err)
	}
	got := argon2.IDKey([]byte(typed), salt, time, memory, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("backup: passphrase mismatch")
	}
	return nil
}
