package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalTarget_PutListDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	tgt, err := NewLocalTarget(dir)
	if err != nil {
		t.Fatalf("NewLocalTarget: %v", err)
	}

	// Put three files with 10ms mtime gaps so List ordering is stable.
	for _, n := range []string{"a.bak", "b.bak", "c.bak"} {
		e, err := tgt.Put(context.Background(), n, bytes.NewReader([]byte("payload-"+n)))
		if err != nil {
			t.Fatalf("Put %s: %v", n, err)
		}
		if e.Name != n {
			t.Errorf("Put Name = %q, want %q", e.Name, n)
		}
		if e.Size != int64(len("payload-"+n)) {
			t.Errorf("Put Size = %d, want %d", e.Size, len("payload-"+n))
		}
		// Stagger the mtime so the sort is deterministic regardless of
		// how fast the loop runs.
		future := time.Now().Add(time.Duration(strings.IndexByte("abc", n[0])) * time.Hour).UTC()
		if err := os.Chtimes(filepath.Join(dir, n), future, future); err != nil {
			t.Fatalf("chtimes %s: %v", n, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	entries, err := tgt.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("List len = %d, want 3", len(entries))
	}
	// Newest-first: c, b, a.
	if entries[0].Name != "c.bak" || entries[1].Name != "b.bak" || entries[2].Name != "a.bak" {
		t.Errorf("List order = [%s, %s, %s], want [c.bak, b.bak, a.bak]",
			entries[0].Name, entries[1].Name, entries[2].Name)
	}

	// Delete middle; List should show 2 with correct order.
	if err := tgt.Delete(context.Background(), "b.bak"); err != nil {
		t.Fatalf("Delete b.bak: %v", err)
	}
	entries, err = tgt.List(context.Background())
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List after delete len = %d, want 2", len(entries))
	}
	if entries[0].Name != "c.bak" || entries[1].Name != "a.bak" {
		t.Errorf("List after delete = [%s, %s], want [c.bak, a.bak]",
			entries[0].Name, entries[1].Name)
	}

	// Missing delete is a no-op (retention sweep semantics).
	if err := tgt.Delete(context.Background(), "nope.bak"); err != nil {
		t.Errorf("Delete missing returned %v, want nil", err)
	}
}

func TestLocalTarget_PutRejectsPathTraversal(t *testing.T) {
	tgt, err := NewLocalTarget(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalTarget: %v", err)
	}
	for _, bad := range []string{"", "../escape", "a/b", `a\b`} {
		if _, err := tgt.Put(context.Background(), bad, bytes.NewReader([]byte("x"))); err == nil {
			t.Errorf("Put(%q) returned nil error, want invalid-name", bad)
		}
		if err := tgt.Delete(context.Background(), bad); err == nil {
			t.Errorf("Delete(%q) returned nil error, want invalid-name", bad)
		}
	}
}

func TestLocalTarget_FilesAre0600(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	tgt, err := NewLocalTarget(dir)
	if err != nil {
		t.Fatalf("NewLocalTarget: %v", err)
	}
	if _, err := tgt.Put(context.Background(), "perm.bak", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "perm.bak"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := st.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("file mode = %o, want 0600 (backup file should be root-only readable)", mode)
	}
}

func TestLocalTarget_NewRejectsEmptyPath(t *testing.T) {
	if _, err := NewLocalTarget(""); err == nil {
		t.Error("NewLocalTarget(\"\") returned nil error, want non-nil")
	}
}

func TestLocalTarget_ListSkipsHiddenAndDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	tgt, err := NewLocalTarget(dir)
	if err != nil {
		t.Fatalf("NewLocalTarget: %v", err)
	}
	if _, err := tgt.Put(context.Background(), "real.bak", bytes.NewReader([]byte("ok"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Hidden file (probe-like) and a subdirectory should be skipped.
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write hidden: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	entries, err := tgt.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "real.bak" {
		t.Errorf("List returned %+v, want only [real.bak]", namesOf(entries))
	}
}

func namesOf(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}
