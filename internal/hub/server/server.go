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

	hubagent "github.com/quanla93/lumen/internal/hub/agent"
	"github.com/quanla93/lumen/internal/hub/alerts"
	"github.com/quanla93/lumen/internal/hub/apikey"
	"github.com/quanla93/lumen/internal/hub/auth"
	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/hubstats"
	"github.com/quanla93/lumen/internal/hub/ingest"
	"github.com/quanla93/lumen/internal/hub/install"
	"github.com/quanla93/lumen/internal/hub/meta"
	"github.com/quanla93/lumen/internal/hub/publicapi"
	"github.com/quanla93/lumen/internal/hub/retention"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/hub/stream"
	"github.com/quanla93/lumen/internal/hub/web"
)

type Config struct {
	Addr                    string
	Dev                     bool
	Version                 string // hub build version (also the latest agent version, same release train)
	StreamInterval          time.Duration
	DBPath                  string
	InstallDir              string
	Secret                  []byte
	RetentionWindow         time.Duration // snapshots older than now-Window are pruned; <=0 disables
	RetentionInterval       time.Duration // sweep cadence; <=0 disables
	RetentionAlertsWindow   time.Duration // resolved alerts + terminal deliveries older than now-Window are pruned; <=0 disables
	AgentInterval           time.Duration // operator policy for agent collection cadence
	DownsampleBucketSize    time.Duration // future cold-tier aggregate width
	DownsampleHotWindow     time.Duration // how long raw SQLite snapshots stay hot before archive
	DownsampleArchiveWindow time.Duration // how long archived Parquet data is retained
	BatchFlushEvery         time.Duration // coalesced INSERT cadence; <=0 → 60s default
	BatchFlushSize          int           // flush early once pending hits this; <=0 → 5000
	AlertEvalInterval       time.Duration // alerts engine eval cadence; <=0 → 15s default
	AdminUsername           string        // env-seeded admin; empty disables seed
	AdminPassword           string        // plaintext at boot, hashed via Argon2id before insert
	Logger                  *slog.Logger
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
		settings.KeyRetentionWindow:         cfg.RetentionWindow.String(),
		settings.KeyRetentionInterval:       cfg.RetentionInterval.String(),
		settings.KeyRetentionAlertsWindow:   cfg.RetentionAlertsWindow.String(),
		settings.KeyAgentInterval:           cfg.AgentInterval.String(),
		settings.KeyDownsampleBucketSize:    cfg.DownsampleBucketSize.String(),
		settings.KeyDownsampleHotWindow:     cfg.DownsampleHotWindow.String(),
		settings.KeyDownsampleArchiveWindow: cfg.DownsampleArchiveWindow.String(),
		settings.KeyAlertEvalInterval:       cfg.AlertEvalInterval.String(),
	}); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}

	go retention.Run(ctx, retention.Config{
		DB:                  db,
		DefaultWindow:       cfg.RetentionWindow,
		DefaultInterval:     cfg.RetentionInterval,
		DefaultAlertsWindow: cfg.RetentionAlertsWindow,
		Logger:              logger.With("subsys", "retention"),
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
	agentPolicyHandler := hubagent.NewPolicyHandler(db, logger)
	streamHandler := stream.New(st, logger, cfg.StreamInterval)
	authHandlers := auth.NewHandlers(db, cfg.Secret, logger)
	hostsHandlers := hosts.NewHandlers(db, st, logger)
	settingsHandlers := settings.NewHandlers(db, logger)
	installHandler := &install.Handler{InstallDir: cfg.InstallDir, Logger: logger}
	metaHandler := meta.New(cfg.Version)
	alertsHandlers := alerts.NewHandlers(db, logger.With("subsys", "alerts"))
	alertsDispatcher := alerts.NewDispatcher(alerts.DispatcherConfig{
		DB:     db,
		Logger: logger.With("subsys", "alerts-dispatch"),
	})
	go alertsDispatcher.Run(ctx)
	alertsEngine := alerts.NewEngine(alerts.Config{
		DB:              db,
		Store:           st,
		Hosts:           alerts.HostsListerFromDB(db),
		Tags:            alerts.TagsListerFromDB(db),
		Dispatcher:      alertsDispatcher,
		DefaultInterval: cfg.AlertEvalInterval,
		Logger:          logger.With("subsys", "alerts"),
	})
	go alertsEngine.Run(ctx)
	hubStatsHandler := &hubstats.Handler{
		DB:        db,
		Store:     st,
		DBPath:    cfg.DBPath,
		Version:   cfg.Version,
		StartedAt: time.Now(),
		Logger:    logger.With("subsys", "hubstats"),
	}
	apiKeysHandlers := apikey.NewHandlers(db, logger.With("subsys", "apikeys"))
	publicAPIHandlers := publicapi.NewHandlers(db, cfg.Version, logger.With("subsys", "publicapi"))
	publicAPILimiter := publicapi.NewLimiter(100, 100) // 100 burst, 100/min refill
	publicAPIAuthn := publicapi.Authn(db, logger.With("subsys", "publicapi"))
	requireSession := auth.RequireSession(cfg.Secret)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	if cfg.Dev {
		r.Use(middleware.Logger)
	}

	r.Get("/healthz", healthz)
	r.Post("/api/ingest", ingestHandler.ServeHTTP)
	r.Get("/api/agent/policy", agentPolicyHandler.ServeHTTP)
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
		r.Get("/api/version", metaHandler.ServeHTTP)
		r.Get("/api/hosts", hostsHandlers.List)
		r.Post("/api/hosts", hostsHandlers.Create)
		r.Delete("/api/hosts/{id}", hostsHandlers.Delete)
		r.Post("/api/hosts/{id}/rotate", hostsHandlers.Rotate)
		r.Get("/api/hosts/{id}/metrics", hostsHandlers.Metrics)
		r.Put("/api/hosts/{id}/tags", hostsHandlers.SetTags)
		r.Post("/api/hosts/{id}/silence", hostsHandlers.Silence)
		r.Delete("/api/hosts/{id}/silence", hostsHandlers.Unsilence)
		r.Get("/api/host-tags", hostsHandlers.ListTagFacets)

		r.Post("/api/account/password", authHandlers.ChangePassword)

		r.Get("/api/settings", settingsHandlers.Get)
		r.Put("/api/settings", settingsHandlers.Put)

		r.Get("/api/admin/hub-stats", hubStatsHandler.ServeHTTP)

		r.Get("/api/apikeys", apiKeysHandlers.List)
		r.Post("/api/apikeys", apiKeysHandlers.Create)
		r.Delete("/api/apikeys/{id}", apiKeysHandlers.Delete)

		r.Get("/api/alerts/rules", alertsHandlers.ListRules)
		r.Post("/api/alerts/rules", alertsHandlers.CreateRule)
		r.Put("/api/alerts/rules/{id}", alertsHandlers.UpdateRule)
		r.Delete("/api/alerts/rules/{id}", alertsHandlers.DeleteRule)

		r.Get("/api/alerts/channels", alertsHandlers.ListChannels)
		r.Post("/api/alerts/channels", alertsHandlers.CreateChannel)
		r.Put("/api/alerts/channels/{id}", alertsHandlers.UpdateChannel)
		r.Delete("/api/alerts/channels/{id}", alertsHandlers.DeleteChannel)
		r.Post("/api/alerts/channels/{id}/test", alertsHandlers.TestChannel)

		r.Get("/api/alerts/events", alertsHandlers.ListEvents)

		r.Get("/api/alerts/deliveries", alertsHandlers.ListDeliveries)
		r.Post("/api/alerts/deliveries/{id}/retry", alertsHandlers.RetryDelivery)

		// Tag inventory — first-class CRUD so hosts + rules pick from a
		// controlled list rather than freeform input. See migration 0012.
		r.Get("/api/tags", alertsHandlers.ListTags)
		r.Post("/api/tags", alertsHandlers.CreateTag)
		r.Put("/api/tags/{key}", alertsHandlers.UpdateTag)
		r.Delete("/api/tags/{key}", alertsHandlers.DeleteTag)
		r.Get("/api/tags/{key}/impact", alertsHandlers.TagImpact)
		r.Post("/api/tags/{key}/values", alertsHandlers.AddTagValue)
		r.Delete("/api/tags/{key}/values/{value}", alertsHandlers.DeleteTagValue)
		r.Get("/api/tags/{key}/values/{value}/impact", alertsHandlers.TagValueImpact)
	})

	// Public Read API — Bearer-key authenticated, per-key rate limited,
	// rich envelope on every response. Versioned at /api/v1/*.
	r.Group(func(r chi.Router) {
		r.Use(publicAPIAuthn)
		r.Use(publicAPILimiter.Middleware)
		r.Get("/api/v1/version", publicAPIHandlers.Version)
		r.With(publicapi.RequireScope(apikey.ScopeReadHosts)).
			Get("/api/v1/hosts", publicAPIHandlers.Hosts)
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
