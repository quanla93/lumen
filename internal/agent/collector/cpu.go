// Package collector reads metrics from the host the agent runs on.
//
// Phase 1 spike: only CPU%. Phase 2 adds RAM, disk, network, load, temps,
// container metrics. Each collector is independent and side-effect free so
// they can run in parallel.
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// CPU samples aggregate CPU usage over the given window and returns the
// percentage in [0, 100]. The window is bounded to a minimum of 100ms so
// gopsutil returns a meaningful delta on the first call.
func CPU(ctx context.Context, window time.Duration) (float64, error) {
	if window < 100*time.Millisecond {
		window = 100 * time.Millisecond
	}
	_ = ctx // gopsutil v4 cpu.Percent is not context-aware; reserved for future use.
	pcts, err := cpu.Percent(window, false)
	if err != nil {
		return 0, fmt.Errorf("cpu.Percent: %w", err)
	}
	if len(pcts) == 0 {
		return 0, fmt.Errorf("cpu.Percent: empty result")
	}
	return pcts[0], nil
}

// CPUPerCore returns one percentage per logical core. The slice length
// equals the number of logical CPUs visible to the agent (cgroup-limited
// inside containers). Window semantics match CPU().
func CPUPerCore(ctx context.Context, window time.Duration) ([]float64, error) {
	if window < 100*time.Millisecond {
		window = 100 * time.Millisecond
	}
	_ = ctx
	pcts, err := cpu.Percent(window, true)
	if err != nil {
		return nil, fmt.Errorf("cpu.Percent per-core: %w", err)
	}
	return pcts, nil
}
