// Package buffer is an on-disk overflow queue that lets the agent survive
// hub downtime without dropping metric frames.
//
// Shape: a single bbolt bucket where every key is a 16-byte tuple of
// (unix-nano big-endian, sequence big-endian) and every value is the
// JSON-encoded ingest envelope. Keys sort lexicographically by time, so a
// forward range scan drains in the order the frames were captured — that
// matters because the hub stores history keyed by ts and we don't want
// late-arriving frames to interleave out of order on the chart.
//
// Lifecycle from the agent's perspective:
//
//   tick → collect envelope
//     │
//     ├─ try Send(env)
//     │    ├─ ok → Drain(N) so a backlog catches up gradually
//     │    └─ err → Enqueue(env); next tick tries again
//     │
//     └─ background prune trims entries older than MaxAge or beyond MaxRows
//
// The buffer is bounded by BOTH a max age (default 24h — older frames
// aren't worth replaying onto a homelab chart) AND a max row count
// (default ~17k = 24h @ 5s tick — keeps the bbolt file small on a
// Raspberry-Pi-class device). Hit either limit and the oldest rows are
// dropped on the next prune.
//
// Corruption: bbolt panics on a malformed file. Open() detects that
// during the initial transaction and renames the file aside with a
// `.corrupt-<unix>` suffix before retrying, so a one-shot disk fault
// doesn't permanently wedge the agent.
package buffer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"

	"github.com/quanla93/lumen/internal/shared/api"
)

const (
	bucketName    = "frames"
	keyLen        = 16 // 8-byte ts-nano + 8-byte seq
	defaultMaxAge = 24 * time.Hour
	defaultMaxRow = 24 * 3600 / 5 // 24h at 5s tick
)

// Config holds the operator-tunable knobs. Zero values default to
// reasonable homelab choices.
type Config struct {
	Path    string
	MaxAge  time.Duration
	MaxRows int
	Logger  *slog.Logger
}

// Buffer is the persistent FIFO queue.
type Buffer struct {
	db      *bbolt.DB
	maxAge  time.Duration
	maxRows int
	logger  *slog.Logger

	seqMu sync.Mutex // serializes monotonic seq assignment across Enqueues
}

// Open creates or reopens the buffer file at cfg.Path. The parent dir
// is created with 0750 if missing; the file itself gets 0600 — the
// envelopes contain hostnames + metrics, not secrets, but conservative
// perms cost nothing.
func Open(cfg Config) (*Buffer, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	maxRows := cfg.MaxRows
	if maxRows <= 0 {
		maxRows = defaultMaxRow
	}

	if cfg.Path == "" {
		return nil, errors.New("buffer.Open: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, fmt.Errorf("mkdir buffer parent: %w", err)
	}

	db, err := openWithCorruptionGuard(cfg.Path, logger)
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &Buffer{
		db:      db,
		maxAge:  maxAge,
		maxRows: maxRows,
		logger:  logger,
	}, nil
}

// openWithCorruptionGuard tries to open the file; if bbolt reports the
// file is invalid, the file is renamed aside and a fresh DB is created.
// We treat corruption as "lose history, keep agent alive" rather than
// "crash and require operator intervention" — the buffer is best-effort
// and a stuck agent loses far more data than a wiped buffer.
func openWithCorruptionGuard(path string, logger *slog.Logger) (*bbolt.DB, error) {
	open := func() (*bbolt.DB, error) {
		return bbolt.Open(path, 0o600, &bbolt.Options{Timeout: 2 * time.Second})
	}
	db, err := open()
	if err == nil {
		return db, nil
	}
	if !errors.Is(err, bbolterrors.ErrInvalid) && !errors.Is(err, bbolterrors.ErrVersionMismatch) {
		return nil, fmt.Errorf("bbolt open: %w", err)
	}
	dst := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
	logger.Warn("buffer file is corrupt, renaming aside and starting fresh",
		"path", path, "moved_to", dst, "err", err)
	if renameErr := os.Rename(path, dst); renameErr != nil {
		return nil, fmt.Errorf("rename corrupt buffer: %w (orig: %v)", renameErr, err)
	}
	return open()
}

// Close releases the bbolt file lock. Safe to call multiple times.
func (b *Buffer) Close() error {
	if b == nil || b.db == nil {
		return nil
	}
	return b.db.Close()
}

// Enqueue appends one envelope to the tail. Returns the assigned key
// only for logging — callers usually ignore it.
func (b *Buffer) Enqueue(env api.IngestRequest) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	key := b.newKey(env.Ts)
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(bucketName)).Put(key, body)
	})
}

// newKey assembles a 16-byte sortable key. The ts half ensures range
// scans drain in capture order even if the system clock ticks
// backwards mid-second; the seq half breaks ties so two envelopes with
// the same ts never overwrite each other.
func (b *Buffer) newKey(ts time.Time) []byte {
	b.seqMu.Lock()
	defer b.seqMu.Unlock()
	// We use bbolt.NextSequence inside an Update tx instead — but
	// allocating a key here keeps Enqueue's single-tx pattern. Use a
	// monotonic process-local counter; collisions across restarts are
	// resolved by the ts prefix.
	seq := nextSeq()
	out := make([]byte, keyLen)
	binary.BigEndian.PutUint64(out[:8], uint64(ts.UnixNano()))
	binary.BigEndian.PutUint64(out[8:], seq)
	return out
}

var (
	seqMu      sync.Mutex
	seqCounter uint64
)

func nextSeq() uint64 {
	seqMu.Lock()
	defer seqMu.Unlock()
	seqCounter++
	return seqCounter
}

// Drain pulls up to max envelopes from the head, calls send for each,
// and deletes on success. Stops on the first send error (the frame
// stays in the queue for the next tick to retry) or context cancel.
// Returns the count successfully shipped.
func (b *Buffer) Drain(
	ctx context.Context, max int,
	send func(context.Context, api.IngestRequest) error,
) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	shipped := 0
	for shipped < max {
		select {
		case <-ctx.Done():
			return shipped, ctx.Err()
		default:
		}
		var key []byte
		var env api.IngestRequest
		err := b.db.View(func(tx *bbolt.Tx) error {
			c := tx.Bucket([]byte(bucketName)).Cursor()
			k, v := c.First()
			if k == nil {
				return errEmpty
			}
			// Copy out of the tx — bbolt's bytes are only valid inside it.
			key = append(key, k...)
			return json.Unmarshal(v, &env)
		})
		if errors.Is(err, errEmpty) {
			return shipped, nil
		}
		if err != nil {
			return shipped, fmt.Errorf("drain peek: %w", err)
		}
		if err := send(ctx, env); err != nil {
			return shipped, err
		}
		// Send succeeded — delete the row. If delete fails we'd
		// re-ship next tick (idempotent on the hub since storage is
		// keyed by host+ts but doesn't dedupe). Log and continue.
		if err := b.db.Update(func(tx *bbolt.Tx) error {
			return tx.Bucket([]byte(bucketName)).Delete(key)
		}); err != nil {
			b.logger.Warn("drain delete failed after successful send", "err", err)
		}
		shipped++
	}
	return shipped, nil
}

var errEmpty = errors.New("buffer empty")

// Size returns the current count. O(1) — bbolt's bucket stats.
func (b *Buffer) Size() (int, error) {
	var n int
	err := b.db.View(func(tx *bbolt.Tx) error {
		n = tx.Bucket([]byte(bucketName)).Stats().KeyN
		return nil
	})
	return n, err
}

// Prune drops entries older than MaxAge or beyond MaxRows. Returns
// the count deleted. Safe to call concurrently with Enqueue/Drain.
func (b *Buffer) Prune() (int, error) {
	cutoff := time.Now().Add(-b.maxAge).UnixNano()
	var deleted int
	err := b.db.Update(func(tx *bbolt.Tx) error {
		bkt := tx.Bucket([]byte(bucketName))
		c := bkt.Cursor()
		// Pass 1: age-based eviction. Walk forward; stop at first key
		// younger than the cutoff. Because the key prefix is ts-nano
		// big-endian, prefix comparison is identical to time comparison.
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if len(k) < 8 {
				continue
			}
			ts := int64(binary.BigEndian.Uint64(k[:8]))
			if ts >= cutoff {
				break
			}
			if err := c.Delete(); err != nil {
				return err
			}
			deleted++
		}
		// Pass 2: count-based eviction. After age pruning, if we're
		// still over MaxRows, drop oldest until we hit the cap.
		extra := bkt.Stats().KeyN - b.maxRows
		if extra > 0 {
			c = bkt.Cursor()
			for k, _ := c.First(); k != nil && extra > 0; k, _ = c.Next() {
				if err := c.Delete(); err != nil {
					return err
				}
				extra--
				deleted++
			}
		}
		return nil
	})
	return deleted, err
}
