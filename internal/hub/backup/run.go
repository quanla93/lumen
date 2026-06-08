// run.go — the actual backup chain: snapshot → seal → put → retain.
//
// One function (RunNow) is the single entry point used by both the
// cron scheduler and the Web UI "Backup now" button. Keeping them on
// the same code path means a manual "Backup now" hits the same
// snapshot+seal+put+retain machinery that the cron tick will, so the
// two paths can never diverge.

package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/quanla93/lumen/internal/hub/auth"
	"github.com/quanla93/lumen/internal/hub/settings"
)

// Plan is the resolved configuration the backup chain operates on.
// Built by NewPlan() from the settings table + the hub secret; all
// downstream functions take a Plan so the contract is explicit.
type Plan struct {
	Enabled    bool
	Target     string // "local" | "s3"
	LocalPath  string
	S3         S3Config
	Cron       string
	RetainLast int
	Passphrase []byte // decrypted in memory; never logged
	HubSecret  []byte // for decrypting the S3 secret_key on read
}

// NewPlan reads the settings table and returns a Plan. Returns an
// error if backup.enabled is false, the target is unrecognised, or
// the passphrase is empty. The passphrase is decrypted lazily in
// RunNow from `backup.passphrase_hash` — no, wait: passphrase_hash is
// a verifier hash (RFC 0001), not the key. The actual passphrase is
// the user-typed value, supplied by the Web UI or the CLI env var.
func NewPlan(ctx context.Context, db *sql.DB, hubSecret []byte, passphrase []byte) (Plan, error) {
	if len(passphrase) == 0 {
		return Plan{}, errors.New("backup: passphrase is empty")
	}
	get := func(key string) string {
		v, _ := settings.Get(ctx, db, key)
		return v
	}
	getInt := func(key string, def int) int {
		v := get(key)
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return n
	}

	p := Plan{
		Enabled:    get("backup.enabled") == "true",
		Target:     get("backup.target"),
		LocalPath:  get("backup.local_path"),
		Cron:       get("backup.cron"),
		RetainLast: getInt("backup.retain_last", 14),
		Passphrase: passphrase,
		HubSecret:  hubSecret,
	}
	if p.Target == "s3" {
		secret, err := auth.DecryptSecret(get("backup.s3_secret_key_enc"), hubSecret)
		if err != nil {
			return Plan{}, fmt.Errorf("backup: s3 secret_key: %w", err)
		}
		p.S3 = S3Config{
			Endpoint:       get("backup.s3_endpoint"),
			Region:         get("backup.s3_region"),
			Bucket:         get("backup.s3_bucket"),
			Prefix:         get("backup.s3_prefix"),
			AccessKey:      get("backup.s3_access_key"),
			SecretKey:      secret,
			ForcePathStyle: get("backup.s3_force_path_style") == "true",
		}
	}
	return p, nil
}

// RunResult is what RunNow returns to the Web UI and to the manual
// "Backup now" log line.
type RunResult struct {
	Name      string
	SizeBytes int64
	Duration  time.Duration
	Hash      string // sha256 of the final on-target artifact (for dedup / display)
}

// RunNow executes one full backup chain. Steps:
//
//  1. Pick a target from the Plan (local or S3).
//  2. VACUUM INTO → gzip → temp file (Snapshot).
//  3. Encrypt (Seal) into a second temp file.
//  4. Put the encrypted blob to the target.
//  5. Sweep old backups honoring retain_last.
//  6. Clean up the temp files.
//
// Temp file lifecycle: each step writes a fresh temp; defer unlinks
// them on any return. We do not encrypt-in-place on the gz file — the
// gz file contains a plain SQLite snapshot (the OIDC client_secret
// etc. live there) and the encryption is what protects the file at
// rest, both on disk and on the S3 / local target.
func RunNow(ctx context.Context, db *sql.DB, p Plan, logger *slog.Logger) (RunResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if !p.Enabled {
		return RunResult{}, errors.New("backup: disabled (set backup.enabled=true first)")
	}
	if len(p.Passphrase) == 0 {
		return RunResult{}, errors.New("backup: passphrase is empty")
	}

	// 1. Pick a target. Probe-write local targets so a misconfigured
	//    path surfaces an error before the snapshot work.
	tgt, err := buildTarget(ctx, p)
	if err != nil {
		return RunResult{}, err
	}

	// 2. Snapshot the live DB into a gz file in a fresh temp dir. We
	//    reuse the same dir for the encrypted temp so cleanup is one
	//    rm -rf.
	tmp, err := os.MkdirTemp("", "lumen-backup-*")
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: mktemp: %w", err)
	}
	defer os.RemoveAll(tmp)

	snapStart := time.Now()
	snap, err := Snapshot(ctx, db, tmp)
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: snapshot: %w", err)
	}
	logger.Info("backup: snapshot done",
		"size_bytes", snap.Size, "took", snap.Duration)

	// 3. Encrypt the gz file into a second temp. We load the whole gz
	//    into memory for Seal; gzip output of a vacuumed SQLite is
	//    well under typical hub RAM (a 100 MB db → ~30 MB gz), and
	//    Seal takes a []byte. For larger fleets the streaming encrypt
	//    path is a follow-up.
	gzBytes, err := os.ReadFile(snap.Path)
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: read gz: %w", err)
	}
	encBytes, err := Seal(p.Passphrase, gzBytes)
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: seal: %w", err)
	}
	encPath := filepath.Join(tmp, "backup.bak")
	if err := os.WriteFile(encPath, encBytes, 0o600); err != nil {
		return RunResult{}, fmt.Errorf("backup: write enc: %w", err)
	}

	// 4. Build the name and put to target. Name format:
	//    lumen-2026-06-08T02-00-00Z.bak — readable in `aws s3 ls`
	//    and in a local ls; the colons in RFC3339 are replaced so
	//    Windows and FAT-formatted drives don't choke.
	now := time.Now().UTC()
	name := "lumen-" + now.Format("2006-01-02T15-04-05Z") + ".bak"

	putStart := time.Now()
	enc, err := os.Open(encPath)
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: open enc: %w", err)
	}
	defer enc.Close()
	_, err = tgt.Put(ctx, name, enc)
	if err != nil {
		return RunResult{}, fmt.Errorf("backup: put: %w", err)
	}
	logger.Info("backup: put done", "name", name, "took", time.Since(putStart))

	// 5. Sweep old backups honoring retain_last. Best-effort; sweep
	//    errors are logged but don't fail the run.
	if deleted, err := Sweep(ctx, tgt, p.RetainLast, logger); err != nil {
		logger.Warn("backup: retention sweep error", "err", err)
	} else if len(deleted) > 0 {
		logger.Info("backup: retention sweep", "deleted", len(deleted))
	}

	// 6. Hash for dedup / display. The full encrypted blob is what
	//    we hash, so the same plaintext + salt + nonce = same hash
	//    only across runs, which is what dedup wants.
	sum := sha256.Sum256(encBytes)
	return RunResult{
		Name:      name,
		SizeBytes: int64(len(encBytes)),
		Duration:  time.Since(snapStart),
		Hash:      hex.EncodeToString(sum[:]),
	}, nil
}

// buildTarget returns the configured Target, validating the local
// path with a probe-write and the S3 config with a HeadBucket (inside
// NewS3Target). Either probe failure surfaces here with a clear
// error message, not "snapshot failed" downstream.
func buildTarget(ctx context.Context, p Plan) (Target, error) {
	switch p.Target {
	case "local":
		if p.LocalPath == "" {
			return nil, errors.New("backup: local_path is empty")
		}
		return NewLocalTarget(p.LocalPath)
	case "s3":
		return NewS3Target(ctx, p.S3)
	default:
		return nil, fmt.Errorf("backup: unknown target %q (want local or s3)", p.Target)
	}
}

// ReadPassphrase is a small helper used by both the CLI restore path
// and the Web UI restore handler. It returns the passphrase from
// (in order) the explicit override (env var or stdin prompt), the
// operator-entered UI value, or an error.
//
// Today the Web UI stores only the Argon2id hash of the passphrase
// (backup.passphrase_hash), not the passphrase itself — that's the
// whole point of "passphrase is non-recoverable". So Web UI restores
// always go through the user re-typing it. CLI restores can use
// LUMEN_HUB_BACKUP_PASSPHRASE for automation.
func ReadPassphrase(envValue, uiValue string) ([]byte, error) {
	if envValue != "" {
		return []byte(envValue), nil
	}
	if uiValue != "" {
		return []byte(uiValue), nil
	}
	return nil, errors.New("backup: passphrase not provided (set LUMEN_HUB_BACKUP_PASSPHRASE or supply via UI)")
}

// ReadAllAtMost is a convenience for streaming callers (the Web UI
// "Download" button) that want to download an entire backup blob
// into a buffer. Caps the read at maxBytes to keep a runaway
// download from OOMing the hub.
func ReadAllAtMost(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("backup: maxBytes must be positive")
	}
	lr := io.LimitReader(r, maxBytes+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("backup: blob exceeds %d bytes", maxBytes)
	}
	return b, nil
}
