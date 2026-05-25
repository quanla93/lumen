// Package ingest exposes the POST /api/ingest handler that agents call.
package ingest

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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
	if err := validate(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	snap := api.HostSnapshot{
		Host:    req.Host,
		Ts:      req.Ts,
		CpuPct:  req.CpuPct,
		RamPct:  req.RamPct,
		SwapPct: req.SwapPct,
		DiskPct: req.DiskPct,
		Load1:   req.Load1,
		Load5:   req.Load5,
		Load15:  req.Load15,
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
	} {
		if p.v < 0 {
			return errors.New(p.name + " must be >= 0")
		}
	}
	return nil
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
