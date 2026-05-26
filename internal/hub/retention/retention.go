// Package retention runs a periodic prune of old rows from the snapshots
// table so a hub's SQLite file stays bounded on HDD.
//
// Phase 2 keeps the policy simple: snapshots older than Window are deleted
// every Interval. Phase 4 will replace the delete with a downsample-and-
// archive job that emits Parquet for the cold tier (see ADR-0001).
package retention

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/lumenhq/lumen/internal/hub/storage"
)

// Run executes one prune immediately, then ticks every Interval until ctx
// is cancelled. Cancellation returns nil — Run is meant to be supervised
// by server.Run, not return an error on shutdown.
//
// All errors are logged but do not stop the loop: retention is best-effort
// and shouldn't take the hub down if SQLite is briefly busy.
type Config struct {
	DB       *sql.DB
	Window   time.Duration // rows older than now-Window are deleted
	Interval time.Duration // sweep cadence
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Window <= 0 || cfg.Interval <= 0 {
		logger.Info("retention disabled (non-positive window or interval)",
			"window", cfg.Window, "interval", cfg.Interval)
		return
	}

	logger.Info("retention loop starting",
		"window", cfg.Window, "interval", cfg.Interval)
	sweep(ctx, cfg, logger)

	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("retention loop stopped")
			return
		case <-t.C:
			sweep(ctx, cfg, logger)
		}
	}
}

func sweep(ctx context.Context, cfg Config, logger *slog.Logger) {
	cutoff := time.Now().Add(-cfg.Window).UTC()
	n, err := storage.DeleteSnapshotsBefore(ctx, cfg.DB, cutoff)
	if err != nil {
		logger.Error("retention sweep failed", "err", err, "cutoff", cutoff)
		return
	}
	if n > 0 {
		logger.Info("retention sweep done", "deleted", n, "cutoff", cutoff)
	} else {
		logger.Debug("retention sweep clean", "cutoff", cutoff)
	}
}
