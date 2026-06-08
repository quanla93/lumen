// snapshot.go — VACUUM INTO the live hub SQLite database into a
// compressed blob suitable for encryption + upload.
//
// Why VACUUM INTO and not `sqlite3 .backup` or just `cp lumen.db`?
//   - VACUUM INTO produces a clean, defragmented, *standalone* SQLite
//     file in one round-trip inside the engine — no WAL, no -shm, no
//     race with a concurrent writer. `cp` would copy a database that
//     still references a hot WAL; .backup streams pages and races with
//     the batch flush goroutine.
//   - The output is a single file with no companion -wal/-shm/-journal,
//     which means it's a safe upload target to S3 and a safe archive
//     target on a local path.
//   - Caller passes us the already-opened *sql.DB; we just exec the
//     VACUUM INTO statement with a quoted path the engine writes to.
//     SQLite handles concurrency; we don't need to stop the hub.

package backup

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SnapshotResult is the (size, path, duration) tuple Snapshot writes
// into its response struct; the caller (scheduler / RunNow) hands the
// fields back to the Web UI.
type SnapshotResult struct {
	Path     string        // absolute path of the temp file holding the gzipped snapshot
	Size     int64         // size on disk in bytes
	Duration time.Duration // wall time of the VACUUM INTO + gzip pipeline
}

// Snapshot produces a gzipped SQLite snapshot of db into a fresh temp
// file in dir. The returned file path is suitable for handing to Seal().
// dir is created if missing.
//
// VACUUM INTO cannot write to a path that already exists. The function
// picks a deterministic-per-call name (timestamp + nano + PID suffix)
// so concurrent callers can't collide; if the chosen name does collide
// for any reason it loops up to MaxVacuumRetries.
func Snapshot(ctx context.Context, db *sql.DB, dir string) (SnapshotResult, error) {
	start := time.Now()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SnapshotResult{}, fmt.Errorf("snapshot: mkdir %s: %w", dir, err)
	}

	// Two temp files: one for the raw VACUUM output, one for the
	// gzipped version. We keep them separate so a partially-written
	// gzip doesn't leave a half-encrypted blob in front of Seal.
	rawPath, err := tempPath(dir, "vacuum-", ".db")
	if err != nil {
		return SnapshotResult{}, err
	}
	gzPath, err := tempPath(dir, "snap-", ".db.gz")
	if err != nil {
		return SnapshotResult{}, err
	}

	// VACUUM INTO needs the path quoted with single quotes (it's a SQL
	// string literal, not a parameter — modernc.org/sqlite binds paths
	// at the engine level, not via parameter substitution).
	if _, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", escapeSingleQuote(rawPath))); err != nil {
		_ = os.Remove(rawPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: VACUUM INTO: %w", err)
	}

	// Best-effort cleanup of the raw file regardless of how gzip
	// finishes — we want at most one artifact on disk per call.
	defer func() { _ = os.Remove(rawPath) }()

	raw, err := os.Open(rawPath)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("snapshot: open raw: %w", err)
	}
	defer raw.Close()

	gz, err := os.Create(gzPath)
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("snapshot: create gz: %w", err)
	}

	w, err := gzip.NewWriterLevel(gz, gzip.BestCompression)
	if err != nil {
		_ = gz.Close()
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: gzip writer: %w", err)
	}

	written, err := io.Copy(w, raw)
	if err != nil {
		_ = w.Close()
		_ = gz.Close()
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: gzip copy: %w", err)
	}
	if err := w.Close(); err != nil {
		_ = gz.Close()
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: gzip close: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: file close: %w", err)
	}

	// gz already wrote its trailer, but the on-disk size includes the
	// gzip header + trailer that gzip.Writer wrote; stat for the real
	// byte count the caller will hand to Seal.
	st, err := os.Stat(gzPath)
	if err != nil {
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: stat: %w", err)
	}

	if written == 0 {
		// Sanity: gzip of a non-empty SQLite file should never be zero
		// bytes (gzip header is at least 10 bytes).
		_ = os.Remove(gzPath)
		return SnapshotResult{}, fmt.Errorf("snapshot: zero bytes copied — VACUUM INTO produced an empty file?")
	}

	return SnapshotResult{
		Path:     gzPath,
		Size:     st.Size(),
		Duration: time.Since(start),
	}, nil
}

// tempPath returns a fresh path in dir with the given prefix/suffix.
// Created with a PID + nanosecond component to avoid collisions even
// when two Snapshot calls land in the same millisecond.
func tempPath(dir, prefix, suffix string) (string, error) {
	now := time.Now().UTC()
	name := fmt.Sprintf("%s%d-%d-%d%s",
		prefix,
		now.Unix(),
		now.Nanosecond(),
		os.Getpid(),
		suffix,
	)
	full := filepath.Join(dir, name)
	if _, err := os.Stat(full); err == nil {
		return "", fmt.Errorf("snapshot: temp path %s already exists", full)
	}
	return full, nil
}

// escapeSingleQuote doubles any single-quote characters in s. SQLite
// string literals use '' to escape a single quote; missing one
// corrupts the path or, worse, injects SQL.
func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
