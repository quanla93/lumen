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
//	LUMEN_HUB_RETENTION_ALERTS_WINDOW (default "720h")     - prune resolved alert events + terminal deliveries older than this; "0" disables
//	LUMEN_HUB_AGENT_INTERVAL      (default "5s")           - runtime policy for agent collection cadence
//	LUMEN_HUB_BATCH_FLUSH_EVERY   (default "60s")          - coalesced snapshot-INSERT cadence (HDD-friendly)
//	LUMEN_HUB_BATCH_FLUSH_SIZE    (default "5000")         - flush early once pending rows hit this count
//	LUMEN_HUB_ALERT_INTERVAL      (default "15s")          - alerts engine evaluation cadence
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
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/quanla93/lumen/internal/hub/backup"
	"github.com/quanla93/lumen/internal/hub/server"
	"github.com/quanla93/lumen/internal/shared/envcfg"
)

// Version is injected at build time via -ldflags "-X main.Version=...".
// The hub and agent ship from the same release train, so this also tells
// the UI the latest agent version a host could be running.
var Version = "dev"

func main() {
	envcfg.Load()

	// --restore=<file> is a one-shot CLI mode: decrypt + integrity-check
	// + atomically swap the live db, then exit. Operators run this
	// from a stopped hub (LUMEN_HUB_BACKUP_PASSPHRASE is the env var
	// they set for automation; --force lets the CLI proceed past the
	// "live -wal is recent" pre-flight check). Mutually exclusive
	// with normal server startup.
	if restorePath, force, ok := parseRestoreFlag(os.Args[1:]); ok {
		if err := runRestore(restorePath, force); err != nil {
			fmt.Fprintln(os.Stderr, "restore failed:", err)
			os.Exit(1)
		}
		return
	}

	addr := envcfg.String("LUMEN_HUB_ADDR", ":8090")
	dev := envcfg.Bool("LUMEN_HUB_DEV", false)
	streamInterval := envcfg.Duration("LUMEN_HUB_STREAM_INTERVAL", 5*time.Second)
	dbPath := envcfg.String("LUMEN_HUB_DB_PATH", "./lumen.db")
	installDir := envcfg.String("LUMEN_HUB_INSTALL_DIR", "")
	secretHex := envcfg.String("LUMEN_HUB_SECRET", "")
	retentionWindow := envcfg.Duration("LUMEN_HUB_RETENTION_WINDOW", 24*time.Hour)
	retentionInterval := envcfg.Duration("LUMEN_HUB_RETENTION_INTERVAL", 1*time.Hour)
	retentionAlertsWindow := envcfg.Duration("LUMEN_HUB_RETENTION_ALERTS_WINDOW", 30*24*time.Hour)
	agentInterval := envcfg.Duration("LUMEN_HUB_AGENT_INTERVAL", 5*time.Second)
	downsampleBucketSize := envcfg.Duration("LUMEN_HUB_DOWNSAMPLE_BUCKET_SIZE", 5*time.Minute)
	downsampleHotWindow := envcfg.Duration("LUMEN_HUB_DOWNSAMPLE_HOT_WINDOW", 24*time.Hour)
	downsampleArchiveWindow := envcfg.Duration("LUMEN_HUB_DOWNSAMPLE_ARCHIVE_WINDOW", 365*24*time.Hour)
	batchFlushEvery := envcfg.Duration("LUMEN_HUB_BATCH_FLUSH_EVERY", 60*time.Second)
	batchFlushSize := envcfg.Int("LUMEN_HUB_BATCH_FLUSH_SIZE", 5000)
	alertInterval := envcfg.Duration("LUMEN_HUB_ALERT_INTERVAL", 15*time.Second)
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
		Addr:                    addr,
		Dev:                     dev,
		Version:                 Version,
		StreamInterval:          streamInterval,
		DBPath:                  dbPath,
		InstallDir:              installDir,
		Secret:                  secret,
		RetentionWindow:         retentionWindow,
		RetentionInterval:       retentionInterval,
		RetentionAlertsWindow:   retentionAlertsWindow,
		AgentInterval:           agentInterval,
		DownsampleBucketSize:    downsampleBucketSize,
		DownsampleHotWindow:     downsampleHotWindow,
		DownsampleArchiveWindow: downsampleArchiveWindow,
		BatchFlushEvery:         batchFlushEvery,
		BatchFlushSize:          batchFlushSize,
		AlertEvalInterval:       alertInterval,
		AdminUsername:           adminUsername,
		AdminPassword:           adminPassword,
		Logger:                  logger,
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

// parseRestoreFlag inspects argv for a `--restore=<file>` token. If
// present, it returns the file path, the --force flag, and ok=true
// so the caller can branch into restore mode and exit.
func parseRestoreFlag(argv []string) (path string, force bool, ok bool) {
	for _, a := range argv {
		if strings.HasPrefix(a, "--restore=") {
			return strings.TrimPrefix(a, "--restore="), false, true
		}
		if a == "--restore" {
			// `--restore <path>` form: caller handles next arg, not us
			// (we treat it as a bare path, simpler than the 2-token
			// form). Kept for future compatibility.
			return "", false, false
		}
		if a == "--force" {
			force = true
		}
	}
	return "", force, false
}

// runRestore is the one-shot CLI path. The hub is expected to be
// stopped (or the operator to have passed --force).
func runRestore(backupPath string, force bool) error {
	passphrase := os.Getenv("LUMEN_HUB_BACKUP_PASSPHRASE")
	if passphrase == "" {
		// If a TTY is attached, prompt with echo off. Operators can
		// always set the env var for automation; the prompt is just
		// the manual-flow convenience.
		t, err := readTTYPassphrase()
		if err != nil {
			return err
		}
		passphrase = t
	}
	dbPath := envcfg.String("LUMEN_HUB_DB_PATH", "./lumen.db")
	if _, err := os.Stat(dbPath); err != nil {
		// Allow restore into a brand-new install: there's no db to
		// replace, so the "previous db" rename step is skipped.
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", dbPath, err)
		}
	}
	res, err := backup.RestoreFile(context.Background(), dbPath, backupPath, []byte(passphrase), force)
	if err != nil {
		return err
	}
	fmt.Printf("restored from %s (size=%d bytes, took=%s)\n", res.Name, res.SizeBytes, res.Duration)
	if res.Predecessor != "" {
		fmt.Printf("previous db preserved at %s\n", res.Predecessor)
	}
	fmt.Println("hub is not restarted automatically; start the service manually to load the new db")
	return nil
}

// readTTYPassphrase prompts the user for a passphrase on stdin with
// echo disabled. golang.org/x/term is the standard way; we use a
// raw-mode ReadPassword so the bytes never round-trip through the
// terminal's line discipline.
func readTTYPassphrase() (string, error) {
	fd := int(os.Stdin.Fd())
	if !termIsTerminal(fd) {
		return "", fmt.Errorf("LUMEN_HUB_BACKUP_PASSPHRASE is empty and stdin is not a TTY; set the env var to use --restore non-interactively")
	}
	fmt.Fprint(os.Stderr, "passphrase: ")
	defer fmt.Fprintln(os.Stderr)
	t, err := termReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	return string(t), nil
}
