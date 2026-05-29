// Package tagutil holds shared validation for tag keys and values.
//
// The same rules apply in two places — hosts.SetTags (per-host
// assignments) and alerts.tags CRUD (inventory). Keeping the rules in
// one place stops them from drifting; importing alerts→hosts (or vice
// versa) would create a cycle.
package tagutil

import (
	"errors"
	"strings"
)

const (
	MaxKeyLen   = 64
	MaxValueLen = 128
)

var (
	ErrKeyRequired  = errors.New("tag key required")
	ErrKeyTooLong   = errors.New("tag key too long (max 64 chars)")
	ErrValueTooLong = errors.New("tag value too long (max 128 chars)")
	ErrKeyInvalid   = errors.New("tag key may only contain letters, digits, '-', '_', '.'")
	ErrValueInvalid = errors.New("tag value contains reserved chars (',' '=')")
)

// NormalizeKey trims whitespace. Returns the cleaned key.
func NormalizeKey(k string) string { return strings.TrimSpace(k) }

// NormalizeValue trims whitespace. Returns the cleaned value.
func NormalizeValue(v string) string { return strings.TrimSpace(v) }

// ValidateKey enforces the format rules. Apply NormalizeKey first.
func ValidateKey(k string) error {
	if k == "" {
		return ErrKeyRequired
	}
	if len(k) > MaxKeyLen {
		return ErrKeyTooLong
	}
	for _, r := range k {
		if !(r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return ErrKeyInvalid
		}
	}
	return nil
}

// ValidateValue enforces the format rules. Empty value is allowed.
func ValidateValue(v string) error {
	if len(v) > MaxValueLen {
		return ErrValueTooLong
	}
	if strings.ContainsAny(v, "=,") {
		return ErrValueInvalid
	}
	return nil
}
