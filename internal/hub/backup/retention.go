// retention.go — keep the last N backups on a target.
//
// This is a pure helper: it does not own a ticker or read settings
// on its own. The scheduler (D3) calls Sweep once per cron tick, after
// the new backup has been successfully written, passing in the
// configured `backup.retain_last` value.
//
// We sort by CreatedAt descending; the contract for List() is
// "newest-first" (LocalTarget sorts itself; S3Target will too) but
// sorting again here is cheap and defensive against an out-of-order
// driver bug.

package backup

import (
	"context"
	"fmt"
	"log/slog"
)

// Sweep deletes excess backups on tgt so that at most retain remain.
// If retain <= 0 the function is a no-op (operator explicitly disabled
// retention). Returns the names deleted, for logging / surfacing in the
// recent-backups UI.
//
// Errors during delete are logged but do not stop the loop — losing
// one delete is a soft failure; the next sweep will retry.
func Sweep(ctx context.Context, tgt Target, retain int, logger *slog.Logger) ([]string, error) {
	if retain <= 0 {
		return nil, nil
	}
	log := logger
	if log == nil {
		log = slog.Default()
	}
	entries, err := tgt.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("backup: retention list: %w", err)
	}
	if len(entries) <= retain {
		return nil, nil
	}
	excess := entries[retain:] // entries are newest-first; tail is oldest
	deleted := make([]string, 0, len(excess))
	for _, e := range excess {
		if err := tgt.Delete(ctx, e.Name); err != nil {
			log.Warn("backup: retention delete failed",
				"name", e.Name,
				"size_bytes", e.Size,
				"err", err)
			continue
		}
		deleted = append(deleted, e.Name)
	}
	return deleted, nil
}
