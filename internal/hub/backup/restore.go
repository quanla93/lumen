// restore.go — verifies a backup blob and writes a restored lumen.db.
//
// Restore is the only place that *writes* to a real DB file, so the
// pre-flight integrity checks live here, not in run.go. The flow is:
//
//  1. Open the blob, parse the 39-byte header.
//  2. Derive the AEAD key with Argon2id (default cost).
//  3. Decrypt; fail cleanly on wrong passphrase / tampered file.
//  4. gunzip into a temp file.
//  5. PRAGMA integrity_check the result.
//  6. Optional pre-flight: refuse if the live -wal file is recent.
//  7. Atomically swap with the live db file; the previous db is
//     preserved as db.before-restore-<ts>.
//  8. Return a summary line; CLI exits, Web UI SIGHUPs the hub.

package backup

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for integrity_check
)

// RestoreResult is what both the CLI and the Web UI return to the
// caller. CreatedAt is the timestamp from inside the original
// backup's CreatedAt (from the target's List), not the wall-clock
// restore time, so the operator sees "restored from <name> created <ts>".
type RestoreResult struct {
	Name        string
	CreatedAt   time.Time
	Source      string    // "local" | "s3" | "<path>" for the CLI file
	RestoredTo  string    // final path of the restored db
	Predecessor string    // path the old db was renamed to (empty if no old db)
	SizeBytes   int64
	Duration    time.Duration
}

// RestoreFile restores a single .bak file from disk to a temp file
// alongside the live db, then atomically swaps. The caller is
// expected to stop the hub first; this function is the canonical /
// safe path (the Web UI button is the convenience path and is
// documented as such in docs/configure/backup.md).
//
// force=true skips the "live -wal is recent" pre-flight check — for
// automation, where the operator has independently confirmed the hub
// is stopped or quiescent.
func RestoreFile(ctx context.Context, dbPath, backupPath string, passphrase []byte, force bool) (RestoreResult, error) {
	start := time.Now()
	if _, err := os.Stat(backupPath); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: restore: source %s: %w", backupPath, err)
	}
	blob, err := os.ReadFile(backupPath)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("backup: restore: read: %w", err)
	}
	res, err := restoreBytes(ctx, dbPath, blob, passphrase, force)
	if err != nil {
		return RestoreResult{}, err
	}
	res.Name = filepath.Base(backupPath)
	res.Source = backupPath
	res.Duration = time.Since(start)
	return res, nil
}

// RestoreFromTarget downloads the named entry from tgt and runs the
// same restoreBytes pipeline. Used by the Web UI "Restore" button:
// the user already has a session, so reading from the configured
// target is the natural way to restore.
func RestoreFromTarget(ctx context.Context, dbPath string, tgt Target, name string, passphrase []byte, force bool) (RestoreResult, error) {
	start := time.Now()
	// Fetch the encrypted blob via List → manual download. We
	// purposely don't add a Target.Get method because the only caller
	// is the Web UI restore button; the canonical restore is CLI
	// (RestoreFile). The S3 path uses a GetObject with a small
	// adapter; the local path is just os.ReadFile.
	blob, createdAt, err := downloadFromTarget(ctx, tgt, name)
	if err != nil {
		return RestoreResult{}, err
	}
	res, err := restoreBytes(ctx, dbPath, blob, passphrase, force)
	if err != nil {
		return RestoreResult{}, err
	}
	res.Name = name
	res.CreatedAt = createdAt
	res.Duration = time.Since(start)
	return res, nil
}

// restoreBytes is the shared body of CLI and Web restore. Verifies +
// decrypts + gunzips + integrity-checks + swaps.
func restoreBytes(ctx context.Context, dbPath string, blob, passphrase []byte, force bool) (RestoreResult, error) {
	res := RestoreResult{RestoredTo: dbPath}

	// 1+2+3. Open + decrypt. OpenWithParams uses DefaultArgon2Params
	// which is what Seal() used to encrypt; the salt+nonce live in
	// the header.
	pt, err := Open(passphrase, blob)
	if err != nil {
		return res, fmt.Errorf("backup: restore: decrypt: %w", err)
	}

	// 4. gunzip into a temp file alongside the live db. Use the
	// same dir so the rename is intra-filesystem (atomic on POSIX).
	dir := filepath.Dir(dbPath)
	tmp, err := os.CreateTemp(dir, "lumen-restore-*.db")
	if err != nil {
		return res, fmt.Errorf("backup: restore: mktemp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// If we never renamed, the temp is leftover — clean it up.
		_ = os.Remove(tmpPath)
	}()

	zr, err := gzip.NewReader(strings.NewReader(string(pt)))
	if err != nil {
		_ = tmp.Close()
		return res, fmt.Errorf("backup: restore: gzip: %w", err)
	}
	if _, err := io.Copy(tmp, zr); err != nil {
		_ = zr.Close()
		_ = tmp.Close()
		return res, fmt.Errorf("backup: restore: gunzip: %w", err)
	}
	if err := zr.Close(); err != nil {
		_ = tmp.Close()
		return res, fmt.Errorf("backup: restore: gzip close: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return res, fmt.Errorf("backup: restore: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return res, fmt.Errorf("backup: restore: close temp: %w", err)
	}

	// 5. PRAGMA integrity_check via a fresh *sql.DB connection. This
	// catches: a non-SQLite file, a half-decrypted blob, a corrupt
	// gzip that happened to round-trip.
	if err := integrityCheck(tmpPath); err != nil {
		return res, fmt.Errorf("backup: restore: integrity check: %w", err)
	}

	// 6. Pre-flight: refuse if a live -wal is recent — the operator
	//    forgot to stop the hub and a writer could race the rename.
	if !force {
		if err := checkLiveWriter(dbPath); err != nil {
			return res, err
		}
	}

	// 7. Atomically swap. The previous db (if any) is preserved as
	//    db.before-restore-<unix>; the operator can hand-restore
	//    from it if the swap was a mistake.
	if _, err := os.Stat(dbPath); err == nil {
		prev := fmt.Sprintf("%s.before-restore-%d", dbPath, time.Now().Unix())
		if err := os.Rename(dbPath, prev); err != nil {
			return res, fmt.Errorf("backup: restore: rename previous: %w", err)
		}
		res.Predecessor = prev
	}
	// Also handle a -wal / -shm leftover from a hub that crashed
	// mid-write — they're now stale (the live db is being replaced).
	// We don't error on a missing -wal; the hub will recreate one
	// on next write.
	for _, side := range []string{"-wal", "-shm", "-journal"} {
		_ = os.Remove(dbPath + side)
	}
	if err := os.Rename(tmpPath, dbPath); err != nil {
		return res, fmt.Errorf("backup: restore: rename new: %w", err)
	}

	// Size of the final file (best effort).
	if st, err := os.Stat(dbPath); err == nil {
		res.SizeBytes = st.Size()
	}
	return res, nil
}

// integrityCheck opens the file with a fresh connection and runs
// PRAGMA integrity_check, which scans the whole db for corruption.
// A pass returns the string "ok"; anything else is reported verbatim.
func integrityCheck(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(OFF)")
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	var v string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&v); err != nil {
		return err
	}
	if v != "ok" {
		return fmt.Errorf("integrity_check = %q", v)
	}
	return nil
}

// checkLiveWriter refuses the restore if a -wal file newer than 5 s
// exists next to the db, which is a strong signal a writer is
// active. Operators can --force past it.
func checkLiveWriter(dbPath string) error {
	const staleness = 5 * time.Second
	for _, side := range []string{"-wal", "-shm"} {
		st, err := os.Stat(dbPath + side)
		if err != nil || st == nil {
			continue
		}
		if time.Since(st.ModTime()) < staleness {
			return fmt.Errorf("backup: restore: %s is fresh (%.0fs old); the hub is likely still running. Stop the hub first or pass --force",
				dbPath+side, time.Since(st.ModTime()).Seconds())
		}
	}
	return nil
}

// downloadFromTarget fetches the encrypted blob for name from tgt.
// Today this uses List to find the entry, then a per-target
// download — but List doesn't return the body, so we add a small
// extension method below. Adding a Target.Get to the interface is
// deferred until a second caller shows up.
func downloadFromTarget(ctx context.Context, tgt Target, name string) ([]byte, time.Time, error) {
	entries, err := tgt.List(ctx)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("backup: restore: list: %w", err)
	}
	for _, e := range entries {
		if e.Name == name {
			blob, err := downloadEntry(ctx, tgt, e)
			if err != nil {
				return nil, time.Time{}, err
			}
			return blob, e.CreatedAt, nil
		}
	}
	return nil, time.Time{}, fmt.Errorf("backup: restore: %s not found on target", name)
}

// downloadEntry is a type switch to the only two concrete targets we
// ship today. When S3Target is added, this grows a third case.
func downloadEntry(ctx context.Context, tgt Target, e Entry) ([]byte, error) {
	switch t := tgt.(type) {
	case *LocalTarget:
		return os.ReadFile(filepath.Join(t.Dir, e.Name))
	case *S3Target:
		return s3Download(ctx, t, e.Name)
	default:
		return nil, errors.New("backup: restore: unsupported target type")
	}
}
