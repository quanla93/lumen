package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Dispatcher decouples the alert engine from HTTP delivery. The engine
// only ever calls Enqueue (cheap: one INSERT); the dispatcher's workers
// poll the notification_deliveries table and send in parallel with
// per-channel serialisation + retry/backoff. This means:
//
//   - A burst of 100 firing alerts inserts 100×N rows in a few hundred
//     ms; the engine ticker is never blocked.
//   - A channel that goes down (Discord 5xx, ntfy unreachable) drains
//     gradually as backoff lets it cool off; other channels are unaffected.
//   - A hub restart resumes from the DB queue — at-least-once delivery
//     for whatever was already enqueued; events that died mid-engine-tick
//     (between event INSERT and delivery INSERT) are the only loss case.
//
// All knobs (poll interval, worker count, max attempts) are runtime
// settings so an operator can dial throughput up/down without redeploy.
type Dispatcher struct {
	cfg DispatcherConfig

	mu           sync.Mutex
	channelLocks map[int64]*sync.Mutex // serialises per-channel dispatch
	policies     map[string]policy     // severity → policy lookup
}

// policyFor returns the retry envelope for a given severity, falling
// back to "warning" for unknown values so we never accidentally lose a
// row to "no policy at all".
func (d *Dispatcher) policyFor(severity string) policy {
	if p, ok := d.policies[severity]; ok {
		return p
	}
	return d.policies["warning"]
}

type DispatcherConfig struct {
	DB           *sql.DB
	HubSecret    []byte // needed for web_push: VAPID private key is encrypted at rest
	// HubURL is the hub's public base URL (e.g. "https://lumen.example.com").
	// Forwarded into DispatchDeps.HubURL so the Slack channel's
	// "View in Lumen" action button has a real target instead of
	// falling back to "https://localhost" (audit finding C3).
	HubURL       string
	Logger       *slog.Logger
	PollInterval time.Duration // how often workers wake to drain
	Workers      int           // parallel goroutines pulling jobs
	// MaxAttempts and Backoff are looked up by severity. Operators
	// can override via SetPolicyForSeverity if a deployment needs
	// different schedules; the defaults below are tuned to:
	//   * critical — fast, give up in ~5 minutes (a 6-hour retry on
	//     a paging-grade alert is useless; the incident is over).
	//   * warning  — moderate, ~7 hours of retries.
	//   * info     — relaxed, same as warning.
	//
	// Reading these per-attempt means an operator change applies to
	// rows already in the queue on their next retry tick.
}

// policy is the per-severity retry envelope.
type policy struct {
	MaxAttempts int
	Backoff     []time.Duration
}

// defaultPolicies are intentionally biased toward "deliver critical fast
// or give up" — a critical alert that retries for 6 hours is a worse
// user experience than no retry at all, because by then the operator is
// either in the war room or has gone home assuming the system is calm.
var defaultPolicies = map[string]policy{
	"critical": {
		MaxAttempts: 4,
		Backoff: []time.Duration{
			5 * time.Second,
			15 * time.Second,
			1 * time.Minute,
			5 * time.Minute,
		},
	},
	"warning": {
		MaxAttempts: 6,
		Backoff: []time.Duration{
			30 * time.Second,
			2 * time.Minute,
			10 * time.Minute,
			1 * time.Hour,
			2 * time.Hour,
			4 * time.Hour,
		},
	},
	"info": {
		MaxAttempts: 6,
		Backoff: []time.Duration{
			30 * time.Second,
			2 * time.Minute,
			10 * time.Minute,
			1 * time.Hour,
			2 * time.Hour,
			4 * time.Hour,
		},
	},
}

func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	return &Dispatcher{
		cfg:          cfg,
		channelLocks: map[int64]*sync.Mutex{},
		policies:     copyPolicies(defaultPolicies),
	}
}

func copyPolicies(src map[string]policy) map[string]policy {
	out := make(map[string]policy, len(src))
	for k, v := range src {
		b := make([]time.Duration, len(v.Backoff))
		copy(b, v.Backoff)
		out[k] = policy{MaxAttempts: v.MaxAttempts, Backoff: b}
	}
	return out
}

// Enqueue persists one (event, channel) row as pending. Called by the
// engine inside its evaluate-tick path. Returns the new delivery id.
// Best-effort: a DB failure here means the engine logs and moves on
// (the alert row still exists in alert_events; the operator can also
// see it in the Active tab even without a notification).
//
// RFC 0004: if the channel has a non-empty digest_window, Enqueue
// also stamps next_flush_at = now + window and rows_count = the
// current count of buffered rows in the same window. The
// claimNext() predicate uses rows_count >= 10 as the early-flush
// trigger; without this update, a burst of 9 events would sit silent
// for the full window even when the 10th arrives 1s later.
func (d *Dispatcher) Enqueue(ctx context.Context, eventID int64, ch Channel, n Notification) (int64, error) {
	payload, err := json.Marshal(n)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	severity := n.Severity
	if severity == "" {
		severity = "warning"
	}

	// Read the channel's current config + compute the window.
	// We re-parse on every enqueue because the operator may have
	// edited the digest_window since the last enqueue; the column
	// here is a per-row snapshot.
	digestWindow := ""
	var nextFlush sql.NullTime
	var rowsCount int
	if cc, perr := ch.ParsedConfig(); perr == nil {
		if win, derr := ParseDigestWindow(cc.DigestWindow); derr == nil && win > 0 {
			digestWindow = cc.DigestWindow
			nextFlush = sql.NullTime{Time: time.Now().UTC().Add(win), Valid: true}
		}
	}

	// RFC 0004 digest-window buffer accounting. We need a write-lock
	// BEFORE the SELECT COUNT so two concurrent enqueues don't
	// both observe count=0. SQLite's default BEGIN DEFERRED only
	// acquires the write lock on the first WRITE; a SELECT before
	// any write is a snapshot read. Audit finding C5.
	//
	// Strategy: BEGIN IMMEDIATE upgrades the BEGIN to take the
	// write lock at txn start. Two concurrent BEGIN IMMEDIATEs
	// serialize on the SQLite writer mutex (with the busy_timeout
	// the hub sets at boot, the wait is bounded).
	//
	// Go's database/sql doesn't expose BEGIN IMMEDIATE through
	// BeginTx, so we drive the transaction on a single Conn from
	// the pool — without pinning the conn, Go's pool would
	// silently hand each ExecContext a different connection and
	// the BEGIN/COMMIT pair would land on different backends
	// (silently broken). Conn() ensures the whole transaction
	// rides one underlying *sqlite.Conn.
	committed := false
	if digestWindow != "" {
		conn, err := d.cfg.DB.Conn(ctx)
		if err != nil {
			return 0, fmt.Errorf("get conn: %w", err)
		}
		defer conn.Close() // returns the conn to the pool
		if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			return 0, fmt.Errorf("begin immediate: %w", err)
		}
		defer func() {
			if !committed {
				_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
			}
		}()
		// Count current buffered rows for the early-flush rule.
		if err := conn.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM notification_deliveries
			 WHERE status = 'pending' AND channel_id = ? AND digest_window = ?`,
			ch.ID, digestWindow,
		).Scan(&rowsCount); err != nil {
			return 0, fmt.Errorf("count pending: %w", err)
		}
		// New row will be #N+1 in the buffer.
		rowsCount++
		// Backfill rows_count on every previously buffered row
		// so claimNext() sees the up-to-date count even when it
		// picks a row other than the latest.
		if rowsCount > 1 {
			if _, err := conn.ExecContext(ctx,
				`UPDATE notification_deliveries
				 SET rows_count = ?
				 WHERE status = 'pending' AND channel_id = ? AND digest_window = ?`,
				rowsCount, ch.ID, digestWindow,
			); err != nil {
				return 0, fmt.Errorf("backfill rows_count: %w", err)
			}
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO notification_deliveries
				(event_id, channel_id, channel_name, channel_type, severity, status, payload,
				 digest_window, next_flush_at, rows_count)
			VALUES (?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?)`,
			eventID, ch.ID, ch.Name, ch.Type, severity, string(payload),
			digestWindow, nextFlush, rowsCount,
		); err != nil {
			return 0, fmt.Errorf("insert delivery: %w", err)
		}
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return 0, fmt.Errorf("commit enqueue: %w", err)
		}
		committed = true
		// committed = true means we return via the success path —
		// LastInsertId is available on the *sql.Conn via a fresh
		// QueryRowContext("SELECT last_insert_rowid()").
		var id int64
		if err := conn.QueryRowContext(ctx, "SELECT last_insert_rowid()").Scan(&id); err != nil {
			return 0, fmt.Errorf("last insert id: %w", err)
		}
		return id, nil
	}

	// Non-digest path: no transaction needed (no count/backfill).
	res, err := d.cfg.DB.ExecContext(ctx, `
		INSERT INTO notification_deliveries
			(event_id, channel_id, channel_name, channel_type, severity, status, payload,
			 digest_window, next_flush_at, rows_count)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?)`,
		eventID, ch.ID, ch.Name, ch.Type, severity, string(payload),
		digestWindow, nextFlush, rowsCount,
	)
	if err != nil {
		return 0, fmt.Errorf("insert delivery: %w", err)
	}
	id, err := res.LastInsertId()
	return id, err
}


// Run boots PollInterval-driven workers. Returns when ctx is cancelled.
// Spawns Workers goroutines; each drains a few jobs per cycle. There's
// no central job channel — workers compete over the DB with an UPDATE-
// then-SELECT pattern under SQLite's WAL, which is the simplest safe
// thing here at homelab scale.
func (d *Dispatcher) Run(ctx context.Context) {
	logger := d.cfg.Logger
	logger.Info("alerts dispatcher starting",
		"poll", d.cfg.PollInterval, "workers", d.cfg.Workers,
		"max_attempts_critical", d.policies["critical"].MaxAttempts,
		"max_attempts_warning", d.policies["warning"].MaxAttempts)

	var wg sync.WaitGroup
	for i := 0; i < d.cfg.Workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			d.workerLoop(ctx, id)
		}(i)
	}
	wg.Wait()
	logger.Info("alerts dispatcher stopped")
}

func (d *Dispatcher) workerLoop(ctx context.Context, id int) {
	// Stagger startups so all workers don't hit the DB on the exact
	// same tick. Negligible cost; helps the SQLite WAL.
	stagger := time.Duration(id) * (d.cfg.PollInterval / time.Duration(d.cfg.Workers+1))
	if stagger > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(stagger):
		}
	}
	t := time.NewTicker(d.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := d.drainOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			d.cfg.Logger.Warn("alerts dispatcher: drain failed", "worker", id, "err", err)
		}
	}
}

// drainOne pulls a single eligible row, runs the dispatch, updates state.
// One row per tick per worker keeps the per-channel mutex story simple
// (no need to batch + reshuffle). Workers × ticks/sec gives effective
// throughput; default 4 workers × 1 Hz = 4 deliveries/sec sustained, more
// than enough for homelab alert volumes.
func (d *Dispatcher) drainOne(ctx context.Context) error {
	row, err := d.claimNext(ctx)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}
	d.process(ctx, *row)
	return nil
}

// pendingRow is the working state for one delivery attempt.
type pendingRow struct {
	ID          int64
	EventID     int64
	ChannelID   int64
	ChannelName string
	ChannelType string
	Severity    string
	Attempts    int
	Payload     string
	// DigestWindow is the channel's digest_window at enqueue time.
	// Empty / "0" = single-shot (today's behaviour). Non-empty
	// = the dispatcher should hold the row until next_flush_at
	// passes, OR rows_count >= 10, whichever comes first.
	DigestWindow string
	RowsCount    int
}

// claimNext is the "find an eligible row and mark it in-flight" step.
// We use status='inflight' as the lease — SQLite WAL serialises the
// UPDATE so two workers can't grab the same row.
//
// next_retry_at IS NULL OR <= now means "fresh row or due for retry".
// ORDER BY id keeps oldest-first delivery.
func (d *Dispatcher) claimNext(ctx context.Context) (*pendingRow, error) {
	tx, err := d.cfg.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var (
		row     pendingRow
		nextStr sql.NullTime
		flushAt sql.NullTime
	)
	// RFC 0004: the WHERE clause now also gates on next_flush_at
	// (digest window expiry) and rows_count (early-flush at N≥10).
	// A row is eligible when:
	//   - no digest window (digest_window = ''), OR
	//   - window expired (next_flush_at IS NULL OR <= now), OR
	//   - rows_count >= 10 (early-flush)
	// We test rows_count first in the predicate so the dispatcher
	// can drain a hot buffer in a single tick.
	err = tx.QueryRowContext(ctx, `
		SELECT id, event_id, channel_id, channel_name, channel_type,
			severity, attempts, payload, next_retry_at,
			COALESCE(digest_window, ''), COALESCE(rows_count, 0), next_flush_at
		FROM notification_deliveries
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= CURRENT_TIMESTAMP)
		  AND (
		    COALESCE(digest_window, '') = ''
		    OR next_flush_at IS NULL
		    OR next_flush_at <= CURRENT_TIMESTAMP
		    OR COALESCE(rows_count, 0) >= 10
		  )
		ORDER BY
			CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
			id
		LIMIT 1`,
	).Scan(&row.ID, &row.EventID, &row.ChannelID, &row.ChannelName,
		&row.ChannelType, &row.Severity, &row.Attempts, &row.Payload, &nextStr,
		&row.DigestWindow, &row.RowsCount, &flushAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Flip status atomically — anyone else competing will get 0 rows.
	res, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status = 'inflight'
		WHERE id = ? AND status = 'pending'`, row.ID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// Lost the race; another worker took it.
		return nil, tx.Commit()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &row, nil
}

// process loads the channel (fresh — config may have been edited),
// holds the per-channel mutex, dispatches, then updates the row.
func (d *Dispatcher) process(ctx context.Context, row pendingRow) {
	ch, err := GetChannel(ctx, d.cfg.DB, row.ChannelID)
	if err != nil {
		// Channel was deleted between enqueue and process — treat as
		// dropped (a real status, not a failure to retry).
		d.markDropped(ctx, row.ID, "channel removed")
		return
	}
	if !ch.Enabled {
		// Operator disabled the channel after enqueue. Drop.
		d.markDropped(ctx, row.ID, "channel disabled")
		return
	}

	// Per-channel serialisation. Prevents us from racing two HTTP calls
	// at the same Discord webhook URL → no self-induced 429s.
	mu := d.channelMutex(row.ChannelID)
	mu.Lock()
	defer mu.Unlock()

	// Deserialise the payload we snapshotted at enqueue time. The engine
	// could have updated the rule's severity etc. since then, but a
	// notification should reflect the moment it was decided, not the
	// moment it's sent.
	var notif Notification
	if err := json.Unmarshal([]byte(row.Payload), &notif); err != nil {
		d.markFailed(ctx, row, 0, "payload decode: "+err.Error())
		return
	}

	dispatchErr := Dispatch(ctx, ch, notif, DispatchDeps{DB: d.cfg.DB, HubSecret: d.cfg.HubSecret, HubURL: d.cfg.HubURL}, d.cfg.Logger)
	if dispatchErr == nil {
		d.markSent(ctx, row.ID)
		d.cfg.Logger.Info("alerts: delivered",
			"delivery_id", row.ID, "channel", ch.Name, "type", ch.Type,
			"attempts", row.Attempts+1)
		return
	}
	d.markFailed(ctx, row, 0, dispatchErr.Error())
}

// markSent finalises a successful delivery row.
func (d *Dispatcher) markSent(ctx context.Context, id int64) {
	_, err := d.cfg.DB.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status = 'sent', sent_at = CURRENT_TIMESTAMP,
			attempts = attempts + 1, error = NULL
		WHERE id = ?`, id)
	if err != nil {
		d.cfg.Logger.Error("alerts dispatcher: mark sent failed", "id", id, "err", err)
	}
}

// markFailed bumps attempts and either schedules a retry or finalises.
// httpStatus is captured opportunistically (0 when not an HTTP error).
// Retry envelope depends on row.Severity: critical gives up fast (~5min),
// warning/info back off longer. See defaultPolicies.
func (d *Dispatcher) markFailed(ctx context.Context, row pendingRow, httpStatus int, errMsg string) {
	pol := d.policyFor(row.Severity)
	attempts := row.Attempts + 1
	if attempts >= pol.MaxAttempts {
		_, err := d.cfg.DB.ExecContext(ctx, `
			UPDATE notification_deliveries
			SET status = 'failed', attempts = ?, http_status = ?, error = ?,
				next_retry_at = NULL
			WHERE id = ?`, attempts, nullInt(httpStatus), errMsg, row.ID)
		if err != nil {
			d.cfg.Logger.Error("alerts dispatcher: mark failed final", "id", row.ID, "err", err)
		}
		d.cfg.Logger.Warn("alerts: delivery exhausted retries",
			"delivery_id", row.ID, "channel", row.ChannelName,
			"severity", row.Severity, "attempts", attempts, "err", errMsg)
		return
	}
	delay := backoffAt(pol.Backoff, attempts)
	next := time.Now().Add(delay).UTC()
	_, err := d.cfg.DB.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status = 'pending', attempts = ?, http_status = ?, error = ?,
			next_retry_at = ?
		WHERE id = ?`, attempts, nullInt(httpStatus), errMsg, next, row.ID)
	if err != nil {
		d.cfg.Logger.Error("alerts dispatcher: mark failed retry", "id", row.ID, "err", err)
	}
	d.cfg.Logger.Warn("alerts: delivery failed, will retry",
		"delivery_id", row.ID, "channel", row.ChannelName,
		"severity", row.Severity, "attempts", attempts,
		"next_retry_in", delay, "err", errMsg)
}

func (d *Dispatcher) markDropped(ctx context.Context, id int64, reason string) {
	_, err := d.cfg.DB.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status = 'dropped', error = ?, next_retry_at = NULL
		WHERE id = ?`, reason, id)
	if err != nil {
		d.cfg.Logger.Error("alerts dispatcher: mark dropped", "id", id, "err", err)
	}
}

// backoffAt picks the wait time for the Nth attempt. Clamps to the last
// slot so an out-of-band attempt count still gets a deterministic delay.
func backoffAt(schedule []time.Duration, attempts int) time.Duration {
	if len(schedule) == 0 {
		return 1 * time.Minute
	}
	idx := attempts - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(schedule) {
		idx = len(schedule) - 1
	}
	return schedule[idx]
}

func (d *Dispatcher) channelMutex(channelID int64) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	mu, ok := d.channelLocks[channelID]
	if !ok {
		mu = &sync.Mutex{}
		d.channelLocks[channelID] = mu
	}
	return mu
}

// PendingCount lets the engine warn the operator if the queue is
// growing (sustained dispatch throughput < enqueue throughput).
func (d *Dispatcher) PendingCount(ctx context.Context) (int, error) {
	var n int
	err := d.cfg.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notification_deliveries WHERE status = 'pending' OR status = 'inflight'`,
	).Scan(&n)
	return n, err
}

func nullInt(v int) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}
