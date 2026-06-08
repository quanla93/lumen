// target.go — the Target interface both the local-filesystem and
// S3-compatible writers implement.
//
// The interface is small on purpose: backup feature ships only the
// write/list/delete primitives it needs today, and anything more
// (multipart, presign, head) lands when a concrete call site asks
// for it. The concrete types — LocalTarget (D2) and S3Target (D3,
// behind the aws-sdk-go-v2 dep) — satisfy it without an adapter.

package backup

import (
	"context"
	"io"
	"time"
)

// Entry is a single backup artifact exposed by List, with the fields
// the Web UI / REST /api/backup/list endpoint cares about.
type Entry struct {
	Name      string    // logical file name, e.g. "lumen-2026-06-05T02-00-00Z.bak"
	Size      int64     // bytes
	CreatedAt time.Time // server-side timestamp of the object (or the file's mtime)
}

// Target is a place backups can be written to and listed/deleted from.
type Target interface {
	// Put writes reader under name. Returns the final size and the
	// server-assigned created_at timestamp (S3 returns its own; local
	// uses the file mtime). The Target owns the lifetime of the
	// resulting artifact — caller does not need to close reader.
	Put(ctx context.Context, name string, reader io.Reader) (Entry, error)

	// List returns all backups sorted newest-first by CreatedAt.
	List(ctx context.Context) ([]Entry, error)

	// Delete removes name. Missing-object errors are non-fatal: the
	// caller (retention sweep) wants "best-effort" semantics.
	Delete(ctx context.Context, name string) error
}
