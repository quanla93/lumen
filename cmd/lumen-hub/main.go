// Lumen hub binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_ADDR             (default ":8090")        - bind address
//	LUMEN_HUB_DEV              (default "false")        - enable verbose request logs + debug logging
//	LUMEN_HUB_STREAM_INTERVAL  (default "5s")           - WS broadcast cadence
//	LUMEN_HUB_DB_PATH          (default "./lumen.db")   - SQLite file location
//
// Phase 1 + 2 endpoints:
//   - GET  /healthz       — liveness probe
//   - POST /api/ingest    — agents push metric snapshots here every ~5s
//   - GET  /api/stream    — WebSocket: pushes current host snapshot every StreamInterval
//
// Snapshots are kept in-memory (hot path for /api/stream) AND archived to
// SQLite at LUMEN_HUB_DB_PATH. Auth + Hosts CRUD land in the next slices.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lumenhq/lumen/internal/hub/server"
	"github.com/lumenhq/lumen/internal/shared/envcfg"
)

func main() {
	envcfg.Load()
	addr := envcfg.String("LUMEN_HUB_ADDR", ":8090")
	dev := envcfg.Bool("LUMEN_HUB_DEV", false)
	streamInterval := envcfg.Duration("LUMEN_HUB_STREAM_INTERVAL", 5*time.Second)
	dbPath := envcfg.String("LUMEN_HUB_DB_PATH", "./lumen.db")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelFor(dev),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, server.Config{
		Addr:           addr,
		Dev:            dev,
		StreamInterval: streamInterval,
		DBPath:         dbPath,
		Logger:         logger,
	}); err != nil {
		logger.Error("hub exited with error", "err", err)
		os.Exit(1)
	}
}

func levelFor(dev bool) slog.Level {
	if dev {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
