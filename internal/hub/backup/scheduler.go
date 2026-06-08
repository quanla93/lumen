// scheduler.go — owns the cron tick + manual trigger of backup runs.
//
// Reads `backup.enabled` + `backup.cron` from the settings table on
// every heartbeat (30s, matching retention's cadence). A cron-tick
// triggers a full RunNow; manual "Backup now" goes through the same
// RunNow entry point so the cron path and the button path can never
// diverge.
//
// Consecutive failures double the backoff delay up to a 4-hour cap
// (RFC 0001 Q4 proposed). One successful run resets the counter. The
// last failure is surfaced as a hub-level alert event so the operator
// gets a single loud signal instead of "cron silently stopped" 30
// hours into the outage.

package backup

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/quanla93/lumen/internal/hub/settings"
)

// RunFunc is the type of a single backup run. The scheduler doesn't
// know how to snapshot + seal + put; the actual work is supplied by
// server.go when it builds the Scheduler. Returning a non-nil error
// means the run failed and counts toward backoff.
type RunFunc func(ctx context.Context) error

// AlertFunc surfaces a human-readable failure message to the alert
// engine. The scheduler calls it after a cron failure; success-path
// calls clear the prior failure.
type AlertFunc func(ctx context.Context, severity, message string)

// Scheduler owns a cron instance and a goroutine that watches the
// settings table for changes. The watch loop is what makes operator
// edits to backup.cron / backup.enabled apply within ~30s without
// restarting the hub.
type Scheduler struct {
	DB *sql.DB

	Run   RunFunc
	Alert AlertFunc

	// Logger is optional; nil falls back to slog.Default.
	Logger *slog.Logger

	parser cron.Parser

	mu             sync.Mutex
	cron           *cron.Cron
	entryID        cron.EntryID // current scheduled entry; EntryID == 0 means "none"
	currentExpr    string       // for logging; "" if no entry
	consecFails    int          // bumped on each failure, reset on success
	backoff        time.Duration // current effective minimum delay between retries
	lastFailureMsg string       // for alert surface
}

// NewScheduler returns a scheduler wired to call run on every cron
// tick. Run is expected to be safe for concurrent use (it is, today:
// the hub has exactly one hub and one backup chain).
func NewScheduler(db *sql.DB, run RunFunc, alert AlertFunc, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		DB:     db,
		Run:    run,
		Alert:  alert,
		Logger: logger,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
	}
}

// Loop blocks until ctx is cancelled, reloading the schedule on each
// heartbeat so the operator can disable / change the cron expression
// from the Settings UI without restarting the hub.
func (s *Scheduler) Loop(ctx context.Context) {
	s.mu.Lock()
	s.cron = cron.New(cron.WithParser(s.parser), cron.WithLogger(cron.DefaultLogger))
	s.cron.Start()
	s.mu.Unlock()

	s.Logger.Info("backup scheduler starting", "heartbeat", HeartbeatInterval)

	tick := time.NewTicker(HeartbeatInterval)
	defer tick.Stop()

	// Eager first reload so a freshly-started hub honors a saved
	// enabled+cron from the settings table.
	s.reload(ctx)

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			if s.cron != nil {
				s.cron.Stop()
			}
			s.mu.Unlock()
			return
		case <-tick.C:
			s.reload(ctx)
		}
	}
}

// reload reads the current settings and either re-installs, replaces,
// or removes the scheduled entry. Called on every heartbeat.
func (s *Scheduler) reload(ctx context.Context) {
	enabled, _ := settings.Get(ctx, s.DB, "backup.enabled")
	expr, _ := settings.Get(ctx, s.DB, "backup.cron")

	wantExpr := ""
	if enabled == "true" && expr != "" {
		wantExpr = expr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if wantExpr == s.currentExpr {
		return // no change; skip the cron.Add churn
	}
	if s.cron == nil {
		return
	}

	// Tear down the previous entry (if any) before installing the new one.
	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
		s.currentExpr = ""
	}

	if wantExpr == "" {
		s.Logger.Info("backup scheduler: disabled or empty cron; no scheduled entry")
		return
	}

	id, err := s.cron.AddFunc(wantExpr, func() {
		s.runOnce(ctx)
	})
	if err != nil {
		s.Logger.Error("backup scheduler: failed to parse cron expression",
			"expr", wantExpr, "err", err)
		return
	}
	s.entryID = id
	s.currentExpr = wantExpr
	s.Logger.Info("backup scheduler: schedule installed", "expr", wantExpr)
}

// runOnce executes one Run, applies success / failure bookkeeping.
// Holds s.mu only briefly so a slow backup doesn't pause schedule
// reloads.
func (s *Scheduler) runOnce(ctx context.Context) {
	if err := s.Run(ctx); err != nil {
		s.recordFailure(ctx, err)
		return
	}
	s.recordSuccess(ctx)
}

// recordFailure bumps the consecutive-fail counter, applies backoff,
// and (after the first failure) raises a hub-level alert event.
func (s *Scheduler) recordFailure(ctx context.Context, err error) {
	s.mu.Lock()
	s.consecFails++
	s.backoff = nextBackoff(s.consecFails, s.backoff)
	msg := err.Error()
	s.lastFailureMsg = msg
	logger := s.Logger
	s.mu.Unlock()

	logger.Error("backup run failed",
		"err", err,
		"consec_fails", s.consecFails,
		"backoff", s.backoff,
	)

	if s.Alert != nil {
		s.Alert(ctx, "warning", "backup: "+msg)
	}
}

// recordSuccess clears the consecutive-fail counter, resets backoff,
// and (if we were previously in a failing state) raises a recovery
// alert event so the operator knows the situation resolved.
func (s *Scheduler) recordSuccess(ctx context.Context) {
	s.mu.Lock()
	wasFailing := s.consecFails > 0
	s.consecFails = 0
	s.backoff = 0
	s.lastFailureMsg = ""
	logger := s.Logger
	s.mu.Unlock()

	logger.Info("backup run succeeded")

	if wasFailing && s.Alert != nil {
		s.Alert(ctx, "info", "backup: recovered after failures")
	}
}

// HeartbeatInterval is the cadence the scheduler reloads its config
// from the settings table. Matches the retention scheduler so a
// Settings UI change to either feature shows up in the other at the
// same wall-clock boundary.
const HeartbeatInterval = 30 * time.Second

// nextBackoff computes the new backoff duration. RFC 0001 Q4 proposed:
// double on each failure up to 4h, then hold at 4h. The function is
// pure so the cron library doesn't have to lock around it.
func nextBackoff(consecFails int, prev time.Duration) time.Duration {
	const cap = 4 * time.Hour
	if consecFails <= 1 {
		return 0
	}
	if prev == 0 {
		prev = time.Minute
	}
	d := prev * 2
	if d > cap {
		d = cap
	}
	return d
}
