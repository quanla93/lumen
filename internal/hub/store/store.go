// Package store keeps the most recent metric snapshot per host in memory,
// plus a short ring of CPU samples per host that the WS stream ships to
// clients so dashboards can render a sparkline immediately on connect
// (no cold-start gap).
//
// Phase 1+2 spike only: no persistence, no SQLite. Phase 2 next slice
// adds a SQLite-backed history layer behind a Query() method; the ring
// stays as the hot/live path.
package store

import (
	"sync"

	"github.com/lumenhq/lumen/internal/shared/api"
)

// SeriesCap is the per-host CPU sample ring length. 120 samples @ 5s tick
// covers the last ten minutes — enough for a sparkline that's meaningful
// on first paint without holding much memory (~1 KB per host).
const SeriesCap = 120

type Store struct {
	mu sync.RWMutex
	m  map[string]api.HostSnapshot
}

func New() *Store {
	return &Store{m: make(map[string]api.HostSnapshot)}
}

// Put records the latest snapshot for snap.Host, appending the CPU value
// to that host's ring (capped at SeriesCap, oldest dropped first).
func (s *Store) Put(snap api.HostSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	series := s.m[snap.Host].CpuSeries
	series = append(series, snap.CpuPct)
	if len(series) > SeriesCap {
		series = series[len(series)-SeriesCap:]
	}
	snap.CpuSeries = series
	s.m[snap.Host] = snap
}

// Snapshot returns a deep-enough copy of every host's latest state.
// CpuSeries slices are aliased — callers should treat them as read-only.
func (s *Store) Snapshot() []api.HostSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]api.HostSnapshot, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	return out
}
