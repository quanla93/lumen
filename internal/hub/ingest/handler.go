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

	h.Store.Put(api.HostSnapshot{
		Host:   req.Host,
		Ts:     req.Ts,
		CpuPct: req.CpuPct,
	})
	h.Logger.Debug("ingest accepted", "host", req.Host, "cpu_pct", req.CpuPct)
	w.WriteHeader(http.StatusNoContent)
}

func validate(req *api.IngestRequest) error {
	if req.Host == "" {
		return errors.New("host required")
	}
	if req.Ts.IsZero() {
		return errors.New("ts required")
	}
	if req.CpuPct < 0 || req.CpuPct > 100 {
		return errors.New("cpu_pct out of [0,100]")
	}
	return nil
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
