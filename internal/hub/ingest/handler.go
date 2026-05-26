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

	"github.com/lumenhq/lumen/internal/hub/hosts"
	"github.com/lumenhq/lumen/internal/hub/storage"
	"github.com/lumenhq/lumen/internal/hub/store"
	"github.com/lumenhq/lumen/internal/shared/api"
)

type Handler struct {
	Store  *store.Store
	DB     *sql.DB
	Logger *slog.Logger
}

func New(s *store.Store, db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{Store: s, DB: db, Logger: logger}
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
		writeErr(w, http.StatusUnauthorized, errors.New("Authorization: Bearer <token> required"))
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
	}
	// In-memory store extends the per-host CpuSeries ring on top of these fields.
	h.Store.Put(snap)

	// Best-effort archive: SQLite failures are logged but don't fail the
	// ingest (the client already got a usable result via the in-memory store).
	if h.DB != nil {
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
	return nil
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
