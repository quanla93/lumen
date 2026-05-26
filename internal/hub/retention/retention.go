// Package retention runs a periodic prune of old rows from the snapshots
// table so a hub's SQLite file stays bounded on HDD.
//
// Phase 2 keeps the policy simple: snapshots older than Window are deleted
// every Interval. Both are read from the `settings` table on every
// heartbeat (30s by default), so operator changes via the Settings UI
// apply within ~30s — no hub restart required. Env vars
// (LUMEN_HUB_RETENTION_{WINDOW,INTERVAL}) seed the initial rows; after
// that the DB wins.
//
// Phase 4 will replace the delete with a downsample-and-archive job that
// emits Parquet for the cold tier (see ADR-0001).
package retention

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/lumenhq/lumen/internal/hub/settings"
	"github.com/lumenhq/lumen/internal/hub/storage"
)

// HeartbeatInterval bounds how quickly a UI setting change takes effect.
// 30s is short enough that operators don't perceive a delay, long enough
// that the table SELECT is essentially free.
const HeartbeatInterval = 30 * time.Second

// Config holds compile-time fallbacks. They're only used when the
// settings table is unreachable or holds malformed values — operator
// edits always win in normal operation.
type Config struct {
	DB              *sql.DB
	DefaultWindow   time.Duration
	DefaultInterval time.Duration
	Logger          *slog.Logger
}

// Run executes one prune immediately, then heartbeats every
// HeartbeatInterval. On each heartbeat it reads the current window +
// interval from the settings table; a sweep fires when
// `time.Since(lastSweep) >= interval`. Setting interval to 0 disables
// sweeps (the loop keeps heartbeating so a later UI change re-enables
// without a restart).
//
// Cancellation returns nil; Run is supervised by server.Run.
// Errors during a sweep are logged but never stop the loop — retention
// is best-effort.
func Run(ctx context.Context, cfg Config) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("retention loop starting",
		"default_window", cfg.DefaultWindow,
		"default_interval", cfg.DefaultInterval,
		"heartbeat", HeartbeatInterval)

	// Eager first sweep so a freshly-started hub doesn't carry stale
	// rows for up to a full interval. Skipped if window <= 0.
	sweep(ctx, cfg, logger)
	lastSweep := time.Now()

	// Track the interval we last logged at so a UI change shows up
	// exactly once in the log (not on every heartbeat).
	var lastLoggedInterval time.Duration

	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("retention loop stopped")
			return
		case <-t.C:
		}

		interval := readInterval(ctx, cfg, logger)
		if interval != lastLoggedInterval && lastLoggedInterval != 0 {
			if interval <= 0 {
				logger.Info("retention disabled by setting change")
			} else {
				logger.Info("retention interval changed",
					"old", lastLoggedInterval, "new", interval)
			}
		}
		lastLoggedInterval = interval

		if interval <= 0 {
			// Disabled — keep heartbeating for the re-enable path.
			continue
		}
		if time.Since(lastSweep) >= interval {
			sweep(ctx, cfg, logger)
			lastSweep = time.Now()
		}
	}
}

func readWindow(ctx context.Context, cfg Config, logger *slog.Logger) time.Duration {
	d, err := settings.GetDuration(ctx, cfg.DB, settings.KeyRetentionWindow, cfg.DefaultWindow)
	if err != nil {
		logger.Warn("retention window read failed, falling back to env default",
			"err", err, "fallback", cfg.DefaultWindow)
		return cfg.DefaultWindow
	}
	return d
}

func readInterval(ctx context.Context, cfg Config, logger *slog.Logger) time.Duration {
	d, err := settings.GetDuration(ctx, cfg.DB, settings.KeyRetentionInterval, cfg.DefaultInterval)
	if err != nil {
		logger.Warn("retention interval read failed, falling back to env default",
			"err", err, "fallback", cfg.DefaultInterval)
		return cfg.DefaultInterval
	}
	return d
}

func sweep(ctx context.Context, cfg Config, logger *slog.Logger) {
	window := readWindow(ctx, cfg, logger)
	if window <= 0 {
		logger.Debug("retention sweep skipped (window <= 0)")
		return
	}
	cutoff := time.Now().Add(-window).UTC()
	n, err := storage.DeleteSnapshotsBefore(ctx, cfg.DB, cutoff)
	if err != nil {
		logger.Error("retention sweep failed", "err", err, "cutoff", cutoff)
		return
	}
	if n > 0 {
		logger.Info("retention sweep done", "deleted", n, "cutoff", cutoff, "window", window)
	} else {
		logger.Debug("retention sweep clean", "cutoff", cutoff, "window", window)
	}
}
