// Package ingest exposes the POST /api/ingest handler that agents call.
package ingest

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/storage"
	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/shared/api"
)

// SnapshotSink absorbs snapshots into the persistent layer. The hub
// uses storage.Batcher (coalesced 60s flushes); tests inject a stub.
type SnapshotSink interface {
	Add(api.HostSnapshot)
}

type Handler struct {
	Store  *store.Store
	DB     *sql.DB
	Sink   SnapshotSink // nil => synchronous per-row insert (kept as fallback)
	Logger *slog.Logger
}

func New(s *store.Store, db *sql.DB, sink SnapshotSink, logger *slog.Logger) *Handler {
	return &Handler{Store: s, DB: db, Sink: sink, Logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req api.IngestRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	// Strict bearer-token auth: a leaked token can't be used to spoof a
	// different host because the token's host name (looked up server-side)
	// overrides whatever the agent put in the body. A request without a
	// token is rejected outright — anonymous ingest was a pre-v0.1 spike
	// affordance and is now closed.
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		writeErr(w, http.StatusUnauthorized, errors.New("missing or malformed Authorization: Bearer <token> header"))
		return
	}
	token := strings.TrimPrefix(hdr, "Bearer ")
	host, err := hosts.VerifyToken(r.Context(), h.DB, token)
	if err != nil {
		if errors.Is(err, hosts.ErrInvalidToken) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid token"))
			return
		}
		h.Logger.Error("token verify failed", "err", err)
		writeErr(w, http.StatusInternalServerError, errors.New("internal error"))
		return
	}
	req.Host = host.Name
	if err := hosts.TouchLastSeen(r.Context(), h.DB, host.ID); err != nil {
		h.Logger.Warn("touch last_seen failed", "err", err, "host", host.Name)
	}

	if err := validate(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := hosts.UpdateSystemMetadata(r.Context(), h.DB, host.ID, req.System); err != nil {
		h.Logger.Warn("system metadata update failed", "err", err, "host", host.Name)
	}

	snap := api.HostSnapshot{
		Host:       req.Host,
		Ts:         req.Ts,
		CpuPct:     req.CpuPct,
		CpuPerCore: req.CpuPerCore,
		RamPct:     req.RamPct,
		SwapPct:    req.SwapPct,
		DiskPct:    req.DiskPct,
		Load1:      req.Load1,
		Load5:      req.Load5,
		Load15:     req.Load15,
		NetRxBps:   req.NetRxBps,
		NetTxBps:   req.NetTxBps,
		DiskRBps:   req.DiskRBps,
		DiskWBps:   req.DiskWBps,
		TempC:      req.TempC,
		Containers: req.Containers,
		System:     req.System,
	}
	// In-memory store extends the per-host CpuSeries ring on top of these fields.
	h.Store.Put(snap)

	// Archive path. Two modes:
	//   - Sink set (production): non-blocking enqueue into the batcher;
	//     a flush every FlushInterval coalesces hundreds of rows into
	//     one transaction (HDD-friendly, see internal/hub/storage/batcher).
	//   - Sink nil (tests / single-host dev): fall back to the original
	//     synchronous INSERT so unit tests that read SQLite immediately
	//     after an ingest don't have to wait for a flush tick.
	if h.Sink != nil {
		h.Sink.Add(snap)
	} else if h.DB != nil {
		if id, err := storage.InsertSnapshot(r.Context(), h.DB, snap); err != nil {
			h.Logger.Warn("snapshot persist failed", "err", err, "host", snap.Host)
		} else {
			h.Logger.Debug("snapshot persisted", "id", id, "host", snap.Host)
		}
	}

	h.Logger.Debug("ingest accepted",
		"host", req.Host, "cpu", req.CpuPct, "ram", req.RamPct,
		"disk", req.DiskPct, "load1", req.Load1)
	w.WriteHeader(http.StatusNoContent)
}

func validate(req *api.IngestRequest) error {
	if req.Host == "" {
		return errors.New("host required")
	}
	if req.Ts.IsZero() {
		return errors.New("ts required")
	}
	for _, p := range []struct {
		name string
		v    float64
	}{
		{"cpu_pct", req.CpuPct}, {"ram_pct", req.RamPct},
		{"swap_pct", req.SwapPct}, {"disk_pct", req.DiskPct},
	} {
		if p.v < 0 || p.v > 100 {
			return errors.New(p.name + " out of [0,100]")
		}
	}
	for _, p := range []struct {
		name string
		v    float64
	}{
		{"load1", req.Load1}, {"load5", req.Load5}, {"load15", req.Load15},
		{"net_rx_bps", req.NetRxBps}, {"net_tx_bps", req.NetTxBps},
		{"disk_r_bps", req.DiskRBps}, {"disk_w_bps", req.DiskWBps},
	} {
		if p.v < 0 {
			return errors.New(p.name + " must be >= 0")
		}
	}
	// Temperature can be sub-zero (cold rooms, missing sensor returning a
	// negative sentinel), but a reading above 150 °C is physically absurd
	// and almost certainly a bad probe.
	if req.TempC < -50 || req.TempC > 150 {
		return errors.New("temp_c out of [-50,150]")
	}
	// Per-core CPU is optional. If present, each value must be a valid
	// percentage. Cap the count at 256 cores to keep envelope sizes
	// bounded against a misbehaving agent.
	if n := len(req.CpuPerCore); n > 256 {
		return errors.New("cpu_per_core too many entries (>256)")
	}
	for i, v := range req.CpuPerCore {
		if v < 0 || v > 100 {
			return fmt.Errorf("cpu_per_core[%d] out of [0,100]", i)
		}
	}
	// Containers list — optional, live-only (not persisted). Cap entries
	// and string lengths so a runaway agent can't OOM the hub by pushing
	// 100k container records.
	if n := len(req.Containers); n > 500 {
		return fmt.Errorf("containers too many entries (>500): %d", n)
	}
	for i := range req.Containers {
		c := &req.Containers[i]
		if len(c.ID) > 64 || len(c.Name) > 128 ||
			len(c.Image) > 256 || len(c.State) > 32 {
			return fmt.Errorf("containers[%d] string field too long", i)
		}
		if c.CpuPct < 0 {
			// Upper bound is "100 * online_cpus" (a container can use all
			// cores). We don't know online_cpus server-side; trust the
			// agent's own cap and just reject negatives.
			return fmt.Errorf("containers[%d].cpu_pct < 0", i)
		}
		if c.MemPct < 0 || c.MemPct > 100 {
			return fmt.Errorf("containers[%d].mem_pct out of [0,100]", i)
		}
	}
	if err := validateSystemMetadata(req.System); err != nil {
		return err
	}
	return nil
}

func validateSystemMetadata(meta api.SystemMetadata) error {
	for _, p := range []struct {
		name  string
		value string
		max   int
	}{
		{"system.os", meta.OS, 128},
		{"system.hostname", meta.Hostname, 128},
		{"system.primary_ip", meta.PrimaryIP, 64},
		{"system.kernel", meta.Kernel, 128},
		{"system.arch", meta.Arch, 32},
		{"system.cpu_model", meta.CPUModel, 256},
		{"system.agent_version", meta.AgentVersion, 64},
	} {
		if len(p.value) > p.max {
			return fmt.Errorf("%s too long (>%d)", p.name, p.max)
		}
	}
	const maxUptimeSeconds = 100 * 365 * 24 * 60 * 60
	if meta.UptimeSeconds > maxUptimeSeconds {
		return errors.New("system.uptime_seconds too large")
	}
	return nil
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
