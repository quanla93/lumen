// Package retention runs a periodic prune of old rows so a hub's SQLite
// file stays bounded on HDD.
//
// Three tables are swept on the same cadence:
//   - snapshots: rows older than retention.window are deleted
//   - alert_events: resolved rows older than retention.delete_alerts_after
//     are deleted (firing rows are kept regardless of age)
//   - notification_deliveries: rows in a terminal state (sent/failed/
//     dropped) older than the same alerts window are deleted. Migration
//     0011 declares ON DELETE CASCADE on event_id, but Open does not
//     enable PRAGMA foreign_keys (an audit of every CASCADE in the schema
//     would have to come first). So this explicit sweep is what actually
//     removes the deliveries — including orphans left over from any
//     pre-foreign-key history.
//
// All three knobs are read from the `settings` table on every heartbeat
// (30s by default), so operator changes via the Settings UI apply within
// ~30s — no hub restart required. Env vars (LUMEN_HUB_RETENTION_{WINDOW,
// INTERVAL,ALERTS_WINDOW}) seed the initial rows; after that the DB wins.
//
// Phase 4 will replace the snapshots delete with a downsample-and-archive
// job that emits Parquet for the cold tier (see ADR-0001). The alerts
// sweep stays a plain delete — there is no cold tier for alert history.
package retention

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
)

// HeartbeatInterval bounds how quickly a UI setting change takes effect.
// 30s is short enough that operators don't perceive a delay, long enough
// that the table SELECT is essentially free.
const HeartbeatInterval = 30 * time.Second

// Config holds compile-time fallbacks. They're only used when the
// settings table is unreachable or holds malformed values — operator
// edits always win in normal operation.
type Config struct {
	DB                  *sql.DB
	DefaultWindow       time.Duration
	DefaultInterval     time.Duration
	DefaultAlertsWindow time.Duration
	Logger              *slog.Logger
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
		"default_alerts_window", cfg.DefaultAlertsWindow,
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

func readAlertsWindow(ctx context.Context, cfg Config, logger *slog.Logger) time.Duration {
	d, err := settings.GetDuration(ctx, cfg.DB, settings.KeyRetentionAlertsWindow, cfg.DefaultAlertsWindow)
	if err != nil {
		logger.Warn("retention alerts window read failed, falling back to env default",
			"err", err, "fallback", cfg.DefaultAlertsWindow)
		return cfg.DefaultAlertsWindow
	}
	return d
}

// sweep runs both the snapshot prune and the alert/delivery prune. The
// two are independent — a failure in one is logged but does not stop the
// other — because the alert sweep was added in v0.4.1 and we don't want
// a regression in the older snapshot path to leave alert tables growing
// (or vice versa).
func sweep(ctx context.Context, cfg Config, logger *slog.Logger) {
	sweepSnapshots(ctx, cfg, logger)
	sweepAlerts(ctx, cfg, logger)
}

func sweepSnapshots(ctx context.Context, cfg Config, logger *slog.Logger) {
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

func sweepAlerts(ctx context.Context, cfg Config, logger *slog.Logger) {
	window := readAlertsWindow(ctx, cfg, logger)
	if window <= 0 {
		logger.Debug("alerts retention sweep skipped (window <= 0)")
		return
	}
	cutoff := time.Now().Add(-window).UTC()
	events, err := storage.DeleteResolvedAlertsBefore(ctx, cfg.DB, cutoff)
	if err != nil {
		logger.Error("alerts retention sweep failed", "err", err, "cutoff", cutoff)
		return
	}
	deliveries, err := storage.DeleteTerminalDeliveriesBefore(ctx, cfg.DB, cutoff)
	if err != nil {
		logger.Error("deliveries retention sweep failed", "err", err, "cutoff", cutoff)
		// Don't return — the event delete already succeeded; we just
		// failed to clean the orphan deliveries on still-firing events.
	}
	if events > 0 || deliveries > 0 {
		logger.Info("alerts retention sweep done",
			"events_deleted", events, "deliveries_deleted", deliveries,
			"cutoff", cutoff, "window", window)
	} else {
		logger.Debug("alerts retention sweep clean", "cutoff", cutoff, "window", window)
	}
}
