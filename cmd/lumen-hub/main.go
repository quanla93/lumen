// Lumen hub binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_ADDR                (default ":8090")        - bind address
//	LUMEN_HUB_DEV                 (default "false")        - enable verbose request logs + debug logging
//	LUMEN_HUB_STREAM_INTERVAL     (default "5s")           - WS broadcast cadence
//	LUMEN_HUB_DB_PATH             (default "./lumen.db")   - SQLite file location
//	LUMEN_HUB_SECRET              (default: random 32B)    - HMAC secret for session JWTs (set explicitly in prod)
//	LUMEN_HUB_INSTALL_DIR         (default "")             - directory holding install.sh + agent binaries; empty disables /install.sh
//	LUMEN_HUB_RETENTION_WINDOW    (default "24h")          - prune snapshots older than this; "0" disables
//	LUMEN_HUB_RETENTION_INTERVAL  (default "1h")           - retention sweep cadence; "0" disables
//	LUMEN_HUB_AGENT_INTERVAL      (default "5s")           - runtime policy for agent collection cadence
//	LUMEN_HUB_BATCH_FLUSH_EVERY   (default "60s")          - coalesced snapshot-INSERT cadence (HDD-friendly)
//	LUMEN_HUB_BATCH_FLUSH_SIZE    (default "5000")         - flush early once pending rows hit this count
//	LUMEN_HUB_ADMIN_USERNAME      (default "")             - seed admin username; both this and password required to enable
//	LUMEN_HUB_ADMIN_PASSWORD      (default "")             - seed admin plaintext password (Argon2id at write time); empty disables seed
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
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quanla93/lumen/internal/hub/server"
	"github.com/quanla93/lumen/internal/shared/envcfg"
)

func main() {
	envcfg.Load()
	addr := envcfg.String("LUMEN_HUB_ADDR", ":8090")
	dev := envcfg.Bool("LUMEN_HUB_DEV", false)
	streamInterval := envcfg.Duration("LUMEN_HUB_STREAM_INTERVAL", 5*time.Second)
	dbPath := envcfg.String("LUMEN_HUB_DB_PATH", "./lumen.db")
	installDir := envcfg.String("LUMEN_HUB_INSTALL_DIR", "")
	secretHex := envcfg.String("LUMEN_HUB_SECRET", "")
	retentionWindow := envcfg.Duration("LUMEN_HUB_RETENTION_WINDOW", 24*time.Hour)
	retentionInterval := envcfg.Duration("LUMEN_HUB_RETENTION_INTERVAL", 1*time.Hour)
	agentInterval := envcfg.Duration("LUMEN_HUB_AGENT_INTERVAL", 5*time.Second)
	batchFlushEvery := envcfg.Duration("LUMEN_HUB_BATCH_FLUSH_EVERY", 60*time.Second)
	batchFlushSize := envcfg.Int("LUMEN_HUB_BATCH_FLUSH_SIZE", 5000)
	adminUsername := envcfg.String("LUMEN_HUB_ADMIN_USERNAME", "")
	adminPassword := envcfg.String("LUMEN_HUB_ADMIN_PASSWORD", "")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelFor(dev),
	}))

	secret, err := resolveSecret(secretHex)
	if err != nil {
		logger.Error("secret bootstrap failed", "err", err)
		os.Exit(1)
	}
	if secretHex == "" {
		logger.Warn("LUMEN_HUB_SECRET not set — generated a random key; sessions will not survive a hub restart. Set this in prod.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, server.Config{
		Addr:              addr,
		Dev:               dev,
		StreamInterval:    streamInterval,
		DBPath:            dbPath,
		InstallDir:        installDir,
		Secret:            secret,
		RetentionWindow:   retentionWindow,
		RetentionInterval: retentionInterval,
		AgentInterval:     agentInterval,
		BatchFlushEvery:   batchFlushEvery,
		BatchFlushSize:    batchFlushSize,
		AdminUsername:     adminUsername,
		AdminPassword:     adminPassword,
		Logger:            logger,
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

// resolveSecret returns the JWT signing secret. If hexStr is non-empty it
// must decode to at least 32 bytes; otherwise a fresh random 32-byte key
// is generated (warned about by the caller — sessions die on restart).
func resolveSecret(hexStr string) ([]byte, error) {
	if hexStr != "" {
		b, err := hex.DecodeString(hexStr)
		if err != nil {
			return nil, err
		}
		if len(b) < 32 {
			return nil, errSecretTooShort
		}
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

var errSecretTooShort = errSecret("LUMEN_HUB_SECRET must be at least 32 random bytes (64 hex chars)")

type errSecret string

func (e errSecret) Error() string { return string(e) }
