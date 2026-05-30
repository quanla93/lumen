// Package settings owns runtime-mutable hub configuration: small key/value
// pairs the operator changes via the Settings UI without restarting the
// process. Env vars (LUMEN_HUB_*) act as defaults; once a row exists in
// the settings table it wins.
//
// Today we expose retention window/interval and agent collection cadence.
// The same store can hold future per-user-preference keys (e.g. default
// dashboard sort) without schema changes.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Keys — kept as constants so handler + retention loop reference the
// same string. Prefer namespacing ("retention.window") so a future
// `lumen-hub settings dump` CLI groups them sensibly.
const (
	KeyRetentionWindow         = "retention.window"
	KeyRetentionInterval       = "retention.interval"
	KeyRetentionAlertsWindow   = "retention.delete_alerts_after"
	KeyAgentInterval           = "agent.interval"
	KeyDownsampleBucketSize    = "downsample.bucket_size"
	KeyDownsampleHotWindow     = "downsample.hot_window"
	KeyDownsampleArchiveWindow = "downsample.archive_window"
	KeyAlertEvalInterval       = "alerts.eval_interval"
)

// Get returns the string value for key, or sql.ErrNoRows if absent.
func Get(ctx context.Context, db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key,
	).Scan(&v)
	return v, err
}

// Set inserts or updates the key's value. Caller is responsible for
// validation; we just persist whatever they pass.
func Set(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

// GetDuration is a small convenience: read key, parse as Go duration.
// If the key doesn't exist, returns fallback with a nil error so the
// caller can use the env default it already knows.
func GetDuration(ctx context.Context, db *sql.DB, key string, fallback time.Duration) (time.Duration, error) {
	v, err := Get(ctx, db, key)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	d, perr := time.ParseDuration(v)
	if perr != nil {
		return 0, fmt.Errorf("settings %q invalid duration %q: %w", key, v, perr)
	}
	return d, nil
}

// EnsureDefaults seeds settings rows from env defaults on first run.
// Called by server.Run after migrations. Re-running is a no-op for any
// key that already has a row — UI changes survive restart.
func EnsureDefaults(ctx context.Context, db *sql.DB, defaults map[string]string) error {
	for k, v := range defaults {
		_, err := Get(ctx, db, k)
		if errors.Is(err, sql.ErrNoRows) {
			if err := Set(ctx, db, k, v); err != nil {
				return fmt.Errorf("seed %s: %w", k, err)
			}
		} else if err != nil {
			return fmt.Errorf("read %s: %w", k, err)
		}
	}
	return nil
}
