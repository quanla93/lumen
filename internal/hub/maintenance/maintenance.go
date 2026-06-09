// Package maintenance implements the alerts engine's "skip notify
// for matching rules during a time window" feature (RFC 0003).
//
// A maintenance window is a time range + a tag-scope selector. The
// alerts engine checks every active window on every tick: if the
// rule's host matches the scope AND the current wall clock is
// inside [start_at, end_at], the engine skips both notification
// dispatch and event-insert. The pre-window events already in the
// delivery queue ship as normal (RFC Q1 proposed: window prevents
// NEW firings only).
//
// One-shot windows in v1 (no recurrence). Edit guards: once a
// window has started, you can only extend end_at — changing
// start_at would silently change which ticks the suppression
// applies to.
package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Window is one row from the maintenance_windows table. ScopeTags
// is a parsed map (the on-disk form is JSON text).
type Window struct {
	ID        int64
	StartAt   time.Time
	EndAt     time.Time
	Reason    string
	ScopeTags map[string]string
	CreatedBy *int64
	CreatedAt time.Time
}

// Cacher is the in-memory cache the alerts engine reads on every
// tick. The cache refreshes on a heartbeat (30 s, same cadence as
// retention + backup) so an operator-added window takes effect
// within ~30 s — fast enough to suppress a scheduled reboot's
// alerts, slow enough to avoid hammering the DB on every tick.
type Cacher struct {
	DB     *sql.DB
	Logger *slog.Logger

	mu    sync.RWMutex
	cache []Window
	loaded time.Time
}

// Heartbeat is the cadence at which the cacher refreshes from the
// DB. Matches retention + backup so the three subsystems refresh
// on the same wall-clock boundary.
const Heartbeat = 30 * time.Second

// Refresh reads the active + upcoming window set from the DB and
// swaps the cache. Called by the alerts engine on its own heartbeat
// (which is the same 30 s cadence).
func (c *Cacher) Refresh(ctx context.Context) error {
	rows, err := c.DB.QueryContext(ctx,
		`SELECT id, start_at, end_at, reason, scope_tags, created_by, created_at
		 FROM maintenance_windows
		 WHERE end_at > ?
		 ORDER BY start_at ASC`,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("maintenance: refresh: %w", err)
	}
	defer rows.Close()

	var out []Window
	for rows.Next() {
		var w Window
		var scopeText string
		var createdBy sql.NullInt64
		if err := rows.Scan(&w.ID, &w.StartAt, &w.EndAt, &w.Reason, &scopeText, &createdBy, &w.CreatedAt); err != nil {
			return fmt.Errorf("maintenance: scan: %w", err)
		}
		w.ScopeTags = map[string]string{}
		if scopeText != "" {
			_ = json.Unmarshal([]byte(scopeText), &w.ScopeTags)
		}
		if createdBy.Valid {
			uid := createdBy.Int64
			w.CreatedBy = &uid
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("maintenance: rows: %w", err)
	}
	c.mu.Lock()
	c.cache = out
	c.loaded = time.Now()
	c.mu.Unlock()
	return nil
}

// ActiveAt returns the list of windows that are currently active
// for a host with the given tag set. Empty slice = no suppression
// applies. Used by the alerts engine on every tick.
func (c *Cacher) ActiveAt(hostTags map[string]string, now time.Time) []Window {
	c.mu.RLock()
	cache := c.cache
	c.mu.RUnlock()

	var out []Window
	for _, w := range cache {
		if now.Before(w.StartAt) || !now.Before(w.EndAt) {
			continue
		}
		if !matchScope(hostTags, w.ScopeTags) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// AllActive builds the host→windows map the alerts engine consumes on
// every tick. Pure: no IO, no DB call — uses the in-memory cache that
// Refresh populates on the cacher's 30s heartbeat. Hosts with no
// currently-active matching window are omitted (the engine treats
// missing keys as "no suppression", same as the empty-map case).
//
// hostTags may be nil — hosts whose tag set is nil only match windows
// whose scope is empty (see ActiveAt's call to matchScope).
//
// hosts may be nil — returns an empty map. Callers typically pass the
// result of alerts.HostsListerFromDB(ctx) so the engine sees the
// canonical host list (not just the ones the in-memory snapshot has
// reported on this tick).
//
// The closure that wraps this method (built in server.go) converts
// each Window to the alerts.MaintenanceWindow slim shape so the
// engine doesn't have to import the maintenance package.
func (c *Cacher) AllActive(hosts []string, hostTags map[string]map[string]string, now time.Time) map[string][]Window {
	if len(hosts) == 0 {
		return nil
	}
	var out map[string][]Window
	for _, host := range hosts {
		wins := c.ActiveAt(hostTags[host], now)
		if len(wins) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string][]Window, len(hosts))
		}
		out[host] = wins
	}
	return out
}

// List returns the cached windows filtered by state (active,
// upcoming, past). Used by the GET /api/maintenance handler.
func (c *Cacher) List(state string, now time.Time) []Window {
	c.mu.RLock()
	cache := c.cache
	c.mu.RUnlock()

	var out []Window
	for _, w := range cache {
		switch state {
		case "active":
			if !now.Before(w.StartAt) && now.Before(w.EndAt) {
				out = append(out, w)
			}
		case "upcoming":
			if now.Before(w.StartAt) {
				out = append(out, w)
			}
		case "past":
			// "past" windows have already been pruned from the cache
			// (the Refresh query is `end_at > now`). We include them
			// here only if the cache hasn't refreshed yet — the
			// past-window list comes from the DB below.
			if !now.Before(w.EndAt) {
				out = append(out, w)
			}
		default:
			out = append(out, w)
		}
	}
	return out
}

// matchScope returns true when the host's tags are a superset of
// (or equal to) the window's scope. Empty scope = matches all
// hosts. Per RFC 0003 §"Maintenance handlers + endpoints": the
// gate is "rule's host matches the scope" — we interpret that as
// the host has every key/value the window requires.
//
// This is the same scope-matching shape the alerts engine already
// uses for tag-based rule selection (see alerts.HostMatchesScope),
// but lifted into the maintenance package so the two stay
// independent and we can test the algorithm in isolation.
func matchScope(hostTags, windowScope map[string]string) bool {
	if len(windowScope) == 0 {
		return true
	}
	for k, v := range windowScope {
		got, ok := hostTags[k]
		if !ok || !strings.EqualFold(got, v) {
			return false
		}
	}
	return true
}

// Create inserts a new window. The created_at + id are returned
// for the response. The caller (handler) is responsible for
// validating scope_tags JSON shape and the start_at < end_at
// invariant before calling.
func Create(ctx context.Context, db *sql.DB, w Window) (int64, error) {
	if !w.EndAt.After(w.StartAt) {
		return 0, errors.New("maintenance: end_at must be after start_at")
	}
	scopeText, err := json.Marshal(w.ScopeTags)
	if err != nil {
		return 0, fmt.Errorf("maintenance: marshal scope: %w", err)
	}
	if w.ScopeTags == nil {
		scopeText = []byte("{}")
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO maintenance_windows
			(start_at, end_at, reason, scope_tags, created_by)
		 VALUES (?, ?, ?, ?, ?)`,
		w.StartAt.UTC(), w.EndAt.UTC(), w.Reason, string(scopeText), w.CreatedBy,
	)
	if err != nil {
		return 0, fmt.Errorf("maintenance: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Update edits an existing window. start_at is immutable once the
// window has begun (per RFC 0003 §"Risks" + RFC 0003 Q1 follow-up);
// end_at may be extended or shortened. Returns ErrEditGuarded if
// the operator tries to mutate start_at on an active window.
func Update(ctx context.Context, db *sql.DB, w Window) error {
	// Load the existing row + check the start_at guard.
	var existingStart, existingEnd time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT start_at, end_at FROM maintenance_windows WHERE id = ?`,
		w.ID,
	).Scan(&existingStart, &existingEnd); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("maintenance: load: %w", err)
	}
	now := time.Now().UTC()
	started := !now.Before(existingStart)
	if started && !w.StartAt.Equal(existingStart) {
		return ErrStartAtLocked
	}
	if !w.EndAt.After(w.StartAt) {
		return errors.New("maintenance: end_at must be after start_at")
	}
	scopeText, err := json.Marshal(w.ScopeTags)
	if err != nil {
		return err
	}
	if w.ScopeTags == nil {
		scopeText = []byte("{}")
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE maintenance_windows
		 SET start_at = ?, end_at = ?, reason = ?, scope_tags = ?
		 WHERE id = ?`,
		w.StartAt.UTC(), w.EndAt.UTC(), w.Reason, string(scopeText), w.ID,
	); err != nil {
		return fmt.Errorf("maintenance: update: %w", err)
	}
	return nil
}

// Delete cancels a window. The cache refresh will drop it on the
// next heartbeat.
func Delete(ctx context.Context, db *sql.DB, id int64) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM maintenance_windows WHERE id = ?`, id); err != nil {
		return fmt.Errorf("maintenance: delete: %w", err)
	}
	return nil
}

// Errors returned by Update. Stable enough to wrap from handlers.
var (
	ErrNotFound      = errors.New("maintenance: window not found")
	ErrStartAtLocked = errors.New("maintenance: start_at is locked once a window has begun (extend end_at instead)")
)
