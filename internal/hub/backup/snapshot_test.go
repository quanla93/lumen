package backup

import (
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// openTestDB creates a tiny SQLite database on disk (the test wants a
// real file because VACUUM INTO requires one — modernc.org/sqlite
// exposes the in-memory :memory: alias but VACUUM INTO doesn't write
// into the connection's own file, and we need a separate path
// destination the engine can resolve). We populate a single settings
// row so the snapshot has at least one user table.
func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "source.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('snapshot.test', 'ok')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return db, dbPath
}

func TestSnapshot_ProducesReadableSQLiteAfterGunzip(t *testing.T) {
	db, _ := openTestDB(t)
	dir := t.TempDir()

	res, err := Snapshot(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if res.Path == "" {
		t.Fatal("Snapshot returned empty path")
	}
	if res.Size <= 0 {
		t.Fatalf("Snapshot size = %d, want > 0", res.Size)
	}
	if res.Duration < 0 {
		t.Fatalf("Snapshot duration negative: %v", res.Duration)
	}

	// Gz path lives in dir; raw path was already cleaned up.
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("gz path not present: %v", err)
	}
	// The raw .db sidecar must NOT remain on disk.
	for _, e := range filepath.Base(res.Path) {
		_ = e
	}
	matches, err := filepath.Glob(filepath.Join(dir, "vacuum-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("raw .db leftover: %v", matches)
	}

	// Gunzip the artifact into a fresh file, then PRAGMA integrity_check
	// the result via a *new* sql.DB connection. This proves the snapshot
	// is a well-formed, encrypted-eligible SQLite file.
	gz, err := os.Open(res.Path)
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer gz.Close()
	zr, err := gzip.NewReader(gz)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()

	out := filepath.Join(dir, "restored.db")
	outF, err := os.Create(out)
	if err != nil {
		t.Fatalf("create restored: %v", err)
	}
	if _, err := io.Copy(outF, zr); err != nil {
		_ = outF.Close()
		t.Fatalf("gunzip copy: %v", err)
	}
	_ = outF.Close()

	restored, err := sql.Open("sqlite", "file:"+out)
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	defer restored.Close()

	var integrity string
	if err := restored.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatalf("integrity_check query: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, want \"ok\"", integrity)
	}

	// And the row we wrote originally must still be there.
	var value string
	if err := restored.QueryRow(`SELECT value FROM settings WHERE key = 'snapshot.test'`).Scan(&value); err != nil {
		t.Fatalf("select restored row: %v", err)
	}
	if value != "ok" {
		t.Fatalf("restored value = %q, want \"ok\"", value)
	}
}

func TestSnapshot_RejectsBadDir(t *testing.T) {
	db, _ := openTestDB(t)
	// A path the OS won't let us mkdir under. On macOS root-owned
	// /private/var/... is off-limits; pick a path nested under a
	// file (not a directory).
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	bad := filepath.Join(blocker, "subdir")
	if _, err := Snapshot(context.Background(), db, bad); err == nil {
		t.Fatal("Snapshot into non-creatable dir returned nil error")
	}
}
