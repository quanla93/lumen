// target_local.go — writes backups to a local filesystem directory.
//
// One writer per directory. The directory is created on construction
// if it doesn't exist; the test for "writable" is a temp file write +
// unlink at the start of Put() so a misconfigured read-only mount
// fails fast with a clear error instead of a half-written backup.

package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalTarget writes backups into a single directory. It does not
// shard or rotate; the scheduler calls Delete via the retention
// sweeper to honor `backup.retain_last`.
type LocalTarget struct {
	Dir string
}

// NewLocalTarget validates the path is usable and returns a writer.
// The directory is created with 0o755 if missing; the validation
// write is then removed before returning.
func NewLocalTarget(dir string) (*LocalTarget, error) {
	if dir == "" {
		return nil, fmt.Errorf("backup: local target: empty directory path")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("backup: local target: mkdir %s: %w", dir, err)
	}
	// Probe writability: stat the dir, then write+unlink a hidden file.
	probe := filepath.Join(dir, ".lumen-write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return nil, fmt.Errorf("backup: local target %s not writable: %w", dir, err)
	}
	_ = os.Remove(probe)
	return &LocalTarget{Dir: dir}, nil
}

// Put writes the reader contents to dir/name and stats the result for
// size + mtime. The file is created 0o600 (root-only readable) — a
// backup contains the OIDC client secret and other sensitive rows in
// plain SQLite; while the *file* is encrypted, the file *permissions*
// shouldn't expose it on a shared host.
func (t *LocalTarget) Put(ctx context.Context, name string, r io.Reader) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if name == "" || strings.ContainsAny(name, "/\\") {
		return Entry{}, fmt.Errorf("backup: local target: invalid name %q (no path separators)", name)
	}
	dst := filepath.Join(t.Dir, name)
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Entry{}, fmt.Errorf("backup: local target: create %s: %w", dst, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(dst)
		return Entry{}, fmt.Errorf("backup: local target: write %s: %w", dst, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dst)
		return Entry{}, fmt.Errorf("backup: local target: close %s: %w", dst, err)
	}
	st, err := os.Stat(dst)
	if err != nil {
		return Entry{}, fmt.Errorf("backup: local target: stat %s: %w", dst, err)
	}
	return Entry{
		Name:      name,
		Size:      st.Size(),
		CreatedAt: st.ModTime().UTC(),
	}, nil
}

// List reads the directory non-recursively and returns files sorted
// newest-first by mtime. Symlinks and directories are skipped; only
// regular files (which is what Put writes) count.
func (t *LocalTarget) List(ctx context.Context) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(t.Dir)
	if err != nil {
		return nil, fmt.Errorf("backup: local target: list %s: %w", t.Dir, err)
	}
	defer f.Close()

	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("backup: local target: readdir %s: %w", t.Dir, err)
	}
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, ".") {
			continue
		}
		full := filepath.Join(t.Dir, n)
		st, err := os.Stat(full)
		if err != nil {
			continue // race with concurrent delete — skip
		}
		if !st.Mode().IsRegular() {
			continue
		}
		out = append(out, Entry{
			Name:      n,
			Size:      st.Size(),
			CreatedAt: st.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Delete removes name. Missing file returns nil — the retention sweep
// wants "best effort" semantics; a missing target isn't an error.
func (t *LocalTarget) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("backup: local target: invalid name %q", name)
	}
	dst := filepath.Join(t.Dir, name)
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: local target: remove %s: %w", dst, err)
	}
	return nil
}
