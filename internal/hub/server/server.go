// Package server wires the chi router, in-memory store, and HTTP listener
// for the Lumen hub.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/lumenhq/lumen/internal/hub/ingest"
	"github.com/lumenhq/lumen/internal/hub/store"
	"github.com/lumenhq/lumen/internal/hub/stream"
)

type Config struct {
	Addr           string
	Dev            bool
	StreamInterval time.Duration
	Logger         *slog.Logger
}

// Run starts the hub HTTP server and blocks until ctx is cancelled.
// On cancellation it gracefully shuts down with a 10s deadline.
func Run(ctx context.Context, cfg Config) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	st := store.New()
	ingestHandler := ingest.New(st, logger)
	streamHandler := stream.New(st, logger, cfg.StreamInterval)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	if cfg.Dev {
		r.Use(middleware.Logger)
	}

	r.Get("/healthz", healthz)
	r.Post("/api/ingest", ingestHandler.ServeHTTP)
	r.Get("/api/stream", streamHandler.ServeHTTP)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("hub listening", "addr", cfg.Addr, "dev", cfg.Dev)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("hub shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("hub stopped")
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
