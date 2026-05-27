// Package server wires the chi router, in-memory store, and HTTP listener
// for the Lumen hub.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/quanla93/lumen/internal/hub/auth"
	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/ingest"
	"github.com/quanla93/lumen/internal/hub/install"
	"github.com/quanla93/lumen/internal/hub/retention"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/hub/stream"
	"github.com/quanla93/lumen/internal/hub/web"
)

type Config struct {
	Addr              string
	Dev               bool
	StreamInterval    time.Duration
	DBPath            string
	InstallDir        string
	Secret            []byte
	RetentionWindow   time.Duration // snapshots older than now-Window are pruned; <=0 disables
	RetentionInterval time.Duration // sweep cadence; <=0 disables
	BatchFlushEvery   time.Duration // coalesced INSERT cadence; <=0 → 60s default
	BatchFlushSize    int           // flush early once pending hits this; <=0 → 5000
	AdminUsername     string        // env-seeded admin; empty disables seed
	AdminPassword     string        // plaintext at boot, hashed via Argon2id before insert
	Logger            *slog.Logger
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

	if err := auth.EnsureUser(ctx, db, cfg.AdminUsername, cfg.AdminPassword, logger); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Seed settings table from env defaults on first run. Once a row
	// exists the UI value wins — env vars become inert (until DB row
	// is deleted manually).
	if err := settings.EnsureDefaults(ctx, db, map[string]string{
		settings.KeyRetentionWindow:   cfg.RetentionWindow.String(),
		settings.KeyRetentionInterval: cfg.RetentionInterval.String(),
	}); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}

	go retention.Run(ctx, retention.Config{
		DB:              db,
		DefaultWindow:   cfg.RetentionWindow,
		DefaultInterval: cfg.RetentionInterval,
		Logger:          logger.With("subsys", "retention"),
	})

	// Batch flush ring: ingest pushes snapshots into the channel here,
	// the goroutine coalesces them into one transaction per FlushEvery.
	// Lifecycle is tied to ctx — on shutdown Run() performs a final
	// flush so in-flight rows survive a SIGTERM.
	batcher := storage.NewBatcher(storage.BatcherConfig{
		DB:            db,
		FlushInterval: cfg.BatchFlushEvery,
		FlushSize:     cfg.BatchFlushSize,
		Logger:        logger.With("subsys", "batcher"),
	})
	batcherDone := make(chan struct{})
	go func() {
		defer close(batcherDone)
		batcher.Run(ctx)
	}()

	st := store.New()
	ingestHandler := ingest.New(st, db, batcher, logger)
	streamHandler := stream.New(st, logger, cfg.StreamInterval)
	authHandlers := auth.NewHandlers(db, cfg.Secret, logger)
	hostsHandlers := hosts.NewHandlers(db, st, logger)
	settingsHandlers := settings.NewHandlers(db, logger)
	installHandler := &install.Handler{InstallDir: cfg.InstallDir, Logger: logger}
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

	// Public install endpoints — script + agent binaries. Both 503 if the
	// hub wasn't built with the binaries staged (e.g. native dev without
	// LUMEN_HUB_INSTALL_DIR set).
	r.Get("/install.sh", installHandler.ServeScript)
	r.Get("/install/{binary}", installHandler.ServeBinary)

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
		r.Get("/api/hosts/{id}/metrics", hostsHandlers.Metrics)

		r.Post("/api/account/password", authHandlers.ChangePassword)

		r.Get("/api/settings", settingsHandlers.Get)
		r.Put("/api/settings", settingsHandlers.Put)
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
	// Wait for the batcher's final flush so the SQLite file is
	// consistent before the process exits. Bounded by the same
	// shutdown deadline as the HTTP server.
	select {
	case <-batcherDone:
	case <-shutdownCtx.Done():
		logger.Warn("batcher did not drain within shutdown deadline — last batch may be lost")
	}
	logger.Info("hub stopped")
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
