package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/load"
)

// Load returns the 1/5/15-minute load averages. Returns an error on Windows
// where load average isn't a kernel-exposed metric — callers should warn and
// fall back to zeros, not fail the whole tick.
func Load(_ context.Context) (l1, l5, l15 float64, err error) {
	avg, err := load.Avg()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("load.Avg: %w", err)
	}
	return avg.Load1, avg.Load5, avg.Load15, nil
}
