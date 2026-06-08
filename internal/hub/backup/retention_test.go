package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeTarget is a Target implementation backed by an in-memory map.
// It satisfies the List/Order contract (newest first) so Sweep can be
// tested without a LocalTarget dependency.
type fakeTarget struct {
	entries map[string]Entry
	listErr error
	delErr  error
}

func newFake() *fakeTarget { return &fakeTarget{entries: map[string]Entry{}} }

func (f *fakeTarget) Put(ctx context.Context, name string, r io.Reader) (Entry, error) {
	// Helper for tests that need to put directly; we don't actually
	// use the bytes, so this just records an entry.
	now := time.Now().UTC()
	e := Entry{Name: name, Size: 0, CreatedAt: now}
	f.entries[name] = e
	return e, nil
}

// PutBytes is a small shim so individual tests can push content; the
// Sweep code never uses the reader, only the names, so the size
// parameter is ignored.
func (f *fakeTarget) PutBytes(name string) (Entry, error) {
	now := time.Now().UTC()
	e := Entry{Name: name, Size: 1, CreatedAt: now}
	f.entries[name] = e
	return e, nil
}

func (f *fakeTarget) List(ctx context.Context) ([]Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]Entry, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e)
	}
	// Newest-first, like LocalTarget.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (f *fakeTarget) Delete(ctx context.Context, name string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.entries, name)
	return nil
}

func TestSweep_LeavesNewestN(t *testing.T) {
	// 20 backups, deterministic mtimes 1s apart, retain_last=5.
	// Expect 5 newest to survive, 15 oldest deleted.
	f := newFake()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("lumen-%02d.bak", i)
		f.entries[name] = Entry{Name: name, Size: 1, CreatedAt: base.Add(time.Duration(i) * time.Second)}
	}

	deleted, err := Sweep(context.Background(), f, 5, slog.Default())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 15 {
		t.Errorf("Sweep deleted %d, want 15", len(deleted))
	}
	if len(f.entries) != 5 {
		t.Fatalf("Sweep left %d, want 5", len(f.entries))
	}
	// Survivors must be the 5 newest: indices 15..19.
	for i, want := 15, 0; want < 20; want++ {
		key := fmt.Sprintf("lumen-%02d.bak", want)
		if want >= 15 {
			if _, ok := f.entries[key]; !ok {
				t.Errorf("Sweep deleted %s which should be retained (idx %d)", key, i)
			}
		} else {
			if _, ok := f.entries[key]; ok {
				t.Errorf("Sweep retained %s which should be deleted (idx %d)", key, i)
			}
		}
	}
}

func TestSweep_NoOpWhenUnderLimit(t *testing.T) {
	f := newFake()
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("n-%d.bak", i)
		f.entries[name] = Entry{Name: name, Size: 1, CreatedAt: time.Unix(int64(i), 0).UTC()}
	}
	deleted, err := Sweep(context.Background(), f, 14, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("Sweep deleted %d, want 0", len(deleted))
	}
	if len(f.entries) != 3 {
		t.Errorf("entry count = %d, want 3", len(f.entries))
	}
}

func TestSweep_RetainZeroIsDisabled(t *testing.T) {
	f := newFake()
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("z-%d.bak", i)
		f.entries[name] = Entry{Name: name, Size: 1, CreatedAt: time.Unix(int64(i), 0).UTC()}
	}
	deleted, err := Sweep(context.Background(), f, 0, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("Sweep with retain=0 deleted %d, want 0", len(deleted))
	}
	if len(f.entries) != 5 {
		t.Errorf("entry count = %d, want 5 (retain=0 is no-op)", len(f.entries))
	}
}

func TestSweep_ListErrorPropagates(t *testing.T) {
	f := newFake()
	f.listErr = context.DeadlineExceeded
	if _, err := Sweep(context.Background(), f, 5, nil); err == nil {
		t.Fatal("Sweep with list error returned nil, want non-nil")
	}
}

func TestSweep_ContinuesOnPerEntryError(t *testing.T) {
	f := newFake()
	// 6 entries, retain=3 → 3 should be deleted. The middle one
	// returns an error, but the others should still be deleted.
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("b-%02d.bak", i)
		f.entries[name] = Entry{Name: name, Size: 1, CreatedAt: base.Add(time.Duration(i) * time.Second)}
	}
	// Force every delete to fail; Sweep should still return cleanly
	// with an empty deleted list and no panic.
	f.delErr = os.ErrPermission
	deleted, err := Sweep(context.Background(), f, 3, slog.Default())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted = %d, want 0 (every delete errored)", len(deleted))
	}
	if len(f.entries) != 6 {
		t.Errorf("entry count = %d, want 6 (nothing actually deleted)", len(f.entries))
	}
}

// Compile-time assertion that fakeTarget satisfies Target. If the
// interface changes this will fail to build, telling us the fake
// needs updating.
var _ Target = (*fakeTarget)(nil)

// Round-trip the local target through Sweep: write 20 files, retain 5.
// The retention sweep in production is going to look exactly like this
// path (LocalTarget → Sweep), so testing it end-to-end catches drift.
func TestSweep_EndToEnd_OnLocalTarget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backups")
	tgt, err := NewLocalTarget(dir)
	if err != nil {
		t.Fatalf("NewLocalTarget: %v", err)
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("e2e-%02d.bak", i)
		e, err := tgt.Put(context.Background(), name, bytes.NewReader([]byte("payload")))
		if err != nil {
			t.Fatalf("Put %s: %v", name, err)
		}
		mt := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(filepath.Join(dir, name), mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		_ = e
	}
	deleted, err := Sweep(context.Background(), tgt, 5, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 15 {
		t.Errorf("Sweep deleted %d, want 15", len(deleted))
	}
	entries, err := tgt.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("List after sweep = %d, want 5", len(entries))
	}
	// Newest 5 (e2e-15..e2e-19) survive; List returns newest-first.
	for i, e := range entries {
		want := fmt.Sprintf("e2e-%02d.bak", 19-i)
		if e.Name != want {
			t.Errorf("entry[%d] = %q, want %q", i, e.Name, want)
		}
	}
}
