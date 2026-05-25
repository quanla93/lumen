// Lumen hub binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_ADDR  (default ":8090")  - bind address
//	LUMEN_HUB_DEV   (default "false")  - enable verbose request logs + debug logging
//
// Phase 1 endpoints:
//   - GET  /healthz       — liveness probe
//   - POST /api/ingest    — agents push metric snapshots here every ~5s
//
// Phase 1.5 adds /api/stream (WebSocket). Phase 2 wires SQLite + auth.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lumenhq/lumen/internal/hub/server"
	"github.com/lumenhq/lumen/internal/shared/envcfg"
)

func main() {
	envcfg.Load()
	addr := envcfg.String("LUMEN_HUB_ADDR", ":8090")
	dev := envcfg.Bool("LUMEN_HUB_DEV", false)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelFor(dev),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, server.Config{Addr: addr, Dev: dev, Logger: logger}); err != nil {
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
