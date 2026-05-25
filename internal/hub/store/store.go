// Package store keeps the most recent metric snapshot per host in memory.
//
// Phase 1 spike only: no persistence, no ring buffer, no eviction. Phase 2
// will batch-flush this state into SQLite every 60s.
package store

import (
	"sync"

	"github.com/lumenhq/lumen/internal/shared/api"
)

type Store struct {
	mu sync.RWMutex
	m  map[string]api.HostSnapshot
}

func New() *Store {
	return &Store{m: make(map[string]api.HostSnapshot)}
}

func (s *Store) Put(snap api.HostSnapshot) {
	s.mu.Lock()
	s.m[snap.Host] = snap
	s.mu.Unlock()
}

func (s *Store) Snapshot() []api.HostSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]api.HostSnapshot, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	return out
}
