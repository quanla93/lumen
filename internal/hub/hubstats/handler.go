// Package hubstats serves an operator-only health snapshot of the hub
// itself: SQLite file size, row counts on the largest tables, Go runtime
// counters, live agent connections, and the alert delivery queue depth.
//
// These are stats only the hub knows — an agent installed on the hub
// host covers CPU/RAM/disk of the LXC, but cannot see the SQLite file
// size or the in-process queue depths.
package hubstats

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/quanla93/lumen/internal/hub/store"
)

// Handler returns a GET handler bound to the resources it needs to read.
type Handler struct {
	DB        *sql.DB
	Store     *store.Store
	DBPath    string
	Version   string
	StartedAt time.Time
	Logger    *slog.Logger

	// cache last-good response for 15s so /admin/hub-stats refreshes are
	// cheap even with COUNT(*) on million-row tables.
	cached atomic.Pointer[cachedStats]
}

type cachedStats struct {
	at   time.Time
	body []byte
}

const cacheTTL = 15 * time.Second

// connectedRecentWindow defines how recently an agent must have ingested
// a snapshot to count as "connected" in the panel. Two-times the typical
// 5s agent interval gives a small grace window for one missed tick.
const connectedRecentWindow = 30 * time.Second

type stats struct {
	Version       string    `json:"version"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	Storage       storage   `json:"storage"`
	Runtime       runtimeS  `json:"runtime"`
	Agents        agentsS   `json:"agents"`
	Deliveries    deliv     `json:"deliveries"`
}

type storage struct {
	DBPath       string           `json:"db_path"`
	DBSizeBytes  int64            `json:"db_size_bytes"`
	WALSizeBytes int64            `json:"wal_size_bytes"`
	Rows         map[string]int64 `json:"rows"`
}

type runtimeS struct {
	GoVersion      string `json:"go_version"`
	Goroutines     int    `json:"goroutines"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	NumGC          uint32 `json:"num_gc"`
}

type agentsS struct {
	Connected  int `json:"connected"`
	Registered int `json:"registered"`
}

type deliv struct {
	Pending  int64 `json:"pending"`
	Inflight int64 `json:"inflight"`
}

// rowTables are scanned with COUNT(*) so the panel can show "is the DB
// growing as expected?". Kept short on purpose — every entry costs a
// table scan on the COUNT, so only the tables operators actually want
// to watch.
var rowTables = []string{
	"snapshots",
	"alert_events",
	"notification_deliveries",
	"hosts",
	"alert_rules",
	"notification_channels",
}

// ServeHTTP renders one JSON document. Response is cached for cacheTTL
// so multiple operators (or a 30s poller) don't repeatedly stack
// COUNT(*) on snapshots.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c := h.cached.Load(); c != nil && time.Since(c.at) < cacheTTL {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(c.body)
		return
	}

	s := h.collect(r.Context())
	body, err := json.Marshal(s)
	if err != nil {
		h.Logger.Error("hubstats marshal", "err", err)
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	h.cached.Store(&cachedStats{at: time.Now(), body: body})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (h *Handler) collect(ctx context.Context) stats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	now := time.Now()
	s := stats{
		Version:       h.Version,
		StartedAt:     h.StartedAt,
		UptimeSeconds: int64(now.Sub(h.StartedAt).Seconds()),
		Storage: storage{
			DBPath:       h.DBPath,
			DBSizeBytes:  fileSize(h.DBPath),
			WALSizeBytes: fileSize(h.DBPath + "-wal"),
			Rows:         map[string]int64{},
		},
		Runtime: runtimeS{
			GoVersion:      runtime.Version(),
			Goroutines:     runtime.NumGoroutine(),
			HeapAllocBytes: ms.HeapAlloc,
			NumGC:          ms.NumGC,
		},
	}

	for _, name := range rowTables {
		if n, err := countRows(ctx, h.DB, name); err == nil {
			s.Storage.Rows[name] = n
		} else {
			h.Logger.Warn("hubstats count failed", "table", name, "err", err)
		}
	}

	// Connected agents = hosts in the in-memory store whose most-recent
	// snapshot timestamp is within connectedRecentWindow. Robust against
	// agents that registered once and went quiet.
	connected := 0
	for _, snap := range h.Store.Snapshot() {
		if now.Sub(snap.Ts) <= connectedRecentWindow {
			connected++
		}
	}
	s.Agents.Connected = connected

	if n, err := countRows(ctx, h.DB, "hosts"); err == nil {
		s.Agents.Registered = int(n)
	}

	if n, err := countDeliveries(ctx, h.DB, "pending"); err == nil {
		s.Deliveries.Pending = n
	}
	if n, err := countDeliveries(ctx, h.DB, "inflight"); err == nil {
		s.Deliveries.Inflight = n
	}

	return s
}

func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var n int64
	// Table names are from a hard-coded allow-list — no injection risk.
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n)
	return n, err
}

func countDeliveries(ctx context.Context, db *sql.DB, status string) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_deliveries WHERE status = ?",
		status,
	).Scan(&n)
	return n, err
}
