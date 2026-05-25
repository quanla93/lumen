// Package ingest exposes the POST /api/ingest handler that agents call.
package ingest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/lumenhq/lumen/internal/hub/store"
	"github.com/lumenhq/lumen/internal/shared/api"
)

type Handler struct {
	Store  *store.Store
	Logger *slog.Logger
}

func New(s *store.Store, logger *slog.Logger) *Handler {
	return &Handler{Store: s, Logger: logger}
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

	// Store.Put extends the per-host CpuSeries ring on top of these fields.
	h.Store.Put(api.HostSnapshot{
		Host:    req.Host,
		Ts:      req.Ts,
		CpuPct:  req.CpuPct,
		RamPct:  req.RamPct,
		SwapPct: req.SwapPct,
		DiskPct: req.DiskPct,
		Load1:   req.Load1,
		Load5:   req.Load5,
		Load15:  req.Load15,
	})
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
