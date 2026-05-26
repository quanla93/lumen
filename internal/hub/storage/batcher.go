// Batcher coalesces InsertSnapshot calls into a single transaction every
// FlushInterval (or sooner if the in-memory queue hits FlushSize).
//
// Why batch:
//   - SQLite WAL with synchronous=NORMAL fsyncs the WAL on commit. One
//     INSERT-per-ingest = one fsync per ingest = N hosts × IOPS per
//     interval. At 200 hosts × 5s tick that's 40 fsyncs/s, plus the
//     spinning-disk seek penalty. Batching every 60s cuts that to ~1
//     transaction per 60s regardless of fleet size.
//   - The WAL stays small. SQLite checkpoints WAL-back-into-main during
//     the COMMIT; a single coalesced commit beats 12k small commits per
//     minute.
//
// The hot path (in-memory store + WebSocket stream) is unaffected — Add
// pushes into a buffered channel and returns immediately. The worst-case
// data loss on a hub crash is one FlushInterval of snapshots, which is
// acceptable for a homelab monitoring tool (the agent itself buffers
// across hub downtime via internal/agent/buffer, so a normal restart
// loses nothing once both sides land in Phase 2-F).
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lumenhq/lumen/internal/shared/api"
)

// BatcherConfig holds the knobs. Defaults are tuned for HDD-backed
// homelab installs (60s flush, generous queue).
type BatcherConfig struct {
	DB            *sql.DB
	FlushInterval time.Duration // default 60s
	FlushSize     int           // default 5000 rows
	QueueSize     int           // default 10000 (channel buffer)
	Logger        *slog.Logger
}

// Batcher absorbs snapshots, flushes them periodically.
type Batcher struct {
	db      *sql.DB
	queue   chan api.HostSnapshot
	flushIn time.Duration
	flushSz int
	logger  *slog.Logger

	// dropped tracks Add calls that found the queue full. Logged on
	// every flush so we never silently lose data.
	dropped uint64
}

// NewBatcher constructs a Batcher. Call Run in a goroutine.
func NewBatcher(cfg BatcherConfig) *Batcher {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 60 * time.Second
	}
	if cfg.FlushSize <= 0 {
		cfg.FlushSize = 5000
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 10000
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Batcher{
		db:      cfg.DB,
		queue:   make(chan api.HostSnapshot, cfg.QueueSize),
		flushIn: cfg.FlushInterval,
		flushSz: cfg.FlushSize,
		logger:  logger,
	}
}

// Add enqueues a snapshot. Non-blocking — if the queue is full the
// snapshot is dropped and a counter is bumped (logged on next flush).
// Dropping on overflow is the right move here: the in-memory store
// already has the latest value for live UI, and stale persisted points
// during a flush stall aren't worth blocking ingest for.
func (b *Batcher) Add(snap api.HostSnapshot) {
	select {
	case b.queue <- snap:
	default:
		b.dropped++
	}
}

// Run blocks until ctx is cancelled. On exit it performs a final flush
// so in-flight snapshots aren't lost across a graceful shutdown.
func (b *Batcher) Run(ctx context.Context) {
	b.logger.Info("batcher starting",
		"flush_interval", b.flushIn, "flush_size", b.flushSz,
		"queue_size", cap(b.queue))

	pending := make([]api.HostSnapshot, 0, b.flushSz)
	t := time.NewTicker(b.flushIn)
	defer t.Stop()

	flush := func(reason string) {
		if len(pending) == 0 {
			return
		}
		start := time.Now()
		if err := b.writeBatch(context.Background(), pending); err != nil {
			b.logger.Error("batch flush failed",
				"err", err, "rows", len(pending), "reason", reason)
		} else {
			b.logger.Debug("batch flushed",
				"rows", len(pending), "reason", reason,
				"took", time.Since(start))
		}
		if b.dropped > 0 {
			b.logger.Warn("batcher dropped snapshots due to full queue",
				"dropped_since_last_flush", b.dropped)
			b.dropped = 0
		}
		pending = pending[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// Drain whatever's still in the channel before exiting.
			for {
				select {
				case snap := <-b.queue:
					pending = append(pending, snap)
				default:
					flush("shutdown")
					b.logger.Info("batcher stopped")
					return
				}
			}
		case <-t.C:
			// Drain everything currently buffered, then flush.
			for done := false; !done; {
				select {
				case snap := <-b.queue:
					pending = append(pending, snap)
					if len(pending) >= b.flushSz {
						done = true
					}
				default:
					done = true
				}
			}
			flush("interval")
		case snap := <-b.queue:
			pending = append(pending, snap)
			if len(pending) >= b.flushSz {
				flush("size")
			}
		}
	}
}

// writeBatch opens one transaction and inserts every snapshot via a
// prepared statement. Any error rolls back the whole batch — partial
// commits would leave gaps that the chart query couldn't reason about.
func (b *Batcher) writeBatch(ctx context.Context, snaps []api.HostSnapshot) error {
	if b.db == nil {
		return errors.New("batcher: nil DB")
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // safe after Commit
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO snapshots
			(host, ts, cpu_pct, ram_pct, swap_pct, disk_pct,
			 load1, load5, load15,
			 net_rx_bps, net_tx_bps, disk_r_bps, disk_w_bps, temp_c)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()
	for i, s := range snaps {
		if _, err := stmt.ExecContext(ctx,
			s.Host, formatTS(s.Ts),
			s.CpuPct, s.RamPct, s.SwapPct, s.DiskPct,
			s.Load1, s.Load5, s.Load15,
			s.NetRxBps, s.NetTxBps, s.DiskRBps, s.DiskWBps, s.TempC,
		); err != nil {
			return fmt.Errorf("exec row %d (host=%s): %w", i, s.Host, err)
		}
	}
	return tx.Commit()
}
