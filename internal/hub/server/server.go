// Package server wires the chi router, in-memory store, and HTTP listener
// for the Lumen hub.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/lumenhq/lumen/internal/hub/auth"
	"github.com/lumenhq/lumen/internal/hub/hosts"
	"github.com/lumenhq/lumen/internal/hub/ingest"
	"github.com/lumenhq/lumen/internal/hub/storage"
	"github.com/lumenhq/lumen/internal/hub/store"
	"github.com/lumenhq/lumen/internal/hub/stream"
	"github.com/lumenhq/lumen/internal/hub/web"
)

type Config struct {
	Addr           string
	Dev            bool
	StreamInterval time.Duration
	DBPath         string
	Secret         []byte
	Logger         *slog.Logger
}

// Run starts the hub HTTP server and blocks until ctx is cancelled.
// On cancellation it gracefully shuts down with a 10s deadline.
func Run(ctx context.Context, cfg Config) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("storage ready", "path", cfg.DBPath)

	st := store.New()
	ingestHandler := ingest.New(st, db, logger)
	streamHandler := stream.New(st, logger, cfg.StreamInterval)
	authHandlers := auth.NewHandlers(db, cfg.Secret, logger)
	hostsHandlers := hosts.NewHandlers(db, logger)
	requireSession := auth.RequireSession(cfg.Secret)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	if cfg.Dev {
		r.Use(middleware.Logger)
	}

	r.Get("/healthz", healthz)
	r.Post("/api/ingest", ingestHandler.ServeHTTP)
	r.Get("/api/stream", streamHandler.ServeHTTP)

	// Auth (public)
	r.Get("/api/setup-status", authHandlers.SetupStatus)
	r.Post("/api/register", authHandlers.Register)
	r.Post("/api/login", authHandlers.Login)
	r.Post("/api/logout", authHandlers.Logout)

	// Auth + Hosts CRUD (session required)
	r.Group(func(r chi.Router) {
		r.Use(requireSession)
		r.Get("/api/me", authHandlers.Me)
		r.Get("/api/hosts", hostsHandlers.List)
		r.Post("/api/hosts", hostsHandlers.Create)
		r.Delete("/api/hosts/{id}", hostsHandlers.Delete)
		r.Post("/api/hosts/{id}/rotate", hostsHandlers.Rotate)
	})

	// Everything else falls to the embedded web bundle (SPA-style), except
	// /api/* which gets an honest 404 — never an HTML page.
	webHandler := web.Handler()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		webHandler.ServeHTTP(w, r)
	})

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
