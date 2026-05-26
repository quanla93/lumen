package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/disk"
)

// DiskIOTotals returns cumulative read/write bytes across every block
// device that's exposing IO counters. Convert to a rate by diffing two
// samples (see Rates).
//
// gopsutil returns a per-device map; we sum it. For homelab disks this is
// the expected single-number view; once we support per-disk drill-down
// we'll surface the map.
func DiskIOTotals(_ context.Context) (readBytes, writeBytes uint64, err error) {
	stats, err := disk.IOCounters()
	if err != nil {
		return 0, 0, fmt.Errorf("disk.IOCounters: %w", err)
	}
	for _, s := range stats {
		readBytes += s.ReadBytes
		writeBytes += s.WriteBytes
	}
	return readBytes, writeBytes, nil
}
