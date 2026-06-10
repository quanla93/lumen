package hosts

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// ShareSweeper periodically deletes expired host_share_tokens rows.
// Once the operator can mint shares (MintShare) the table grows
// without bound unless something prunes the expired rows — at
// homelab scale the cost is negligible but the table makes every
// ListHostShares query scan stale rows.
//
// Loop runs at SweepInterval until ctx is cancelled. The first
// sweep happens immediately so a freshly-restarted hub catches up
// on any rows that expired while it was down.
type ShareSweeper struct {
	DB     *sql.DB
	Logger *slog.Logger
}

// SweepInterval is the cadence at which the sweeper prunes
// expired shares. Hourly matches the share TTL granularity
// (1h..720h) and keeps the table scan cost trivial.
const SweepInterval = 1 * time.Hour

// Loop runs the sweeper until ctx is cancelled.
func (s *ShareSweeper) Loop(ctx context.Context) {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	logger := s.Logger
	if err := s.sweepOnce(ctx); err != nil {
		logger.Warn("share sweep failed", "err", err)
	}
	t := time.NewTicker(SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.sweepOnce(ctx); err != nil {
				logger.Warn("share sweep failed", "err", err)
			}
		}
	}
}

func (s *ShareSweeper) sweepOnce(ctx context.Context) error {
	n, err := SweepExpiredShares(ctx, s.DB, time.Now().UTC())
	if err != nil {
		return err
	}
	if n > 0 {
		s.Logger.Info("share sweep pruned expired rows", "count", n)
	}
	return nil
}
