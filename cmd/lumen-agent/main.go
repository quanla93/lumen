// Package main is the entry point for the Lumen agent binary.
//
// Phase 1.1/1.2 status: skeleton only — reads one CPU sample via gopsutil and
// exits. Phase 1.4 wires the 5s collection loop and HTTP POST to the hub.
//
// Chosen libraries for the agent (locked in ACTION_PLAN Phase 1):
//   - Metrics:   github.com/shirou/gopsutil/v4
//   - Buffer:    go.etcd.io/bbolt              (added when offline buffer needed, Phase 2)
package main

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func main() {
	pcts, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		log.Fatalf("lumen-agent: cpu sample failed: %v", err)
	}
	log.Printf("lumen-agent: skeleton build — CPU sample = %.2f%% (collection loop wired in Phase 1.4)", pcts[0])
}
