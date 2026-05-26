package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/net"
)

// NetTotals returns the cumulative bytes received/transmitted across all
// physical and virtual interfaces visible to the agent. Convert to a rate
// by diffing two samples (see Rates).
//
// On hosts with many bridges / veth pairs (typical for Docker hosts) this
// over-counts because traffic that's both forwarded and received shows up
// on the bridge AND on the underlying interface. Pre-v0.1 we accept the
// over-count; a follow-up can filter to a configured interface list.
func NetTotals(_ context.Context) (rxBytes, txBytes uint64, err error) {
	stats, err := net.IOCounters(false) // pernic=false → one summed entry
	if err != nil {
		return 0, 0, fmt.Errorf("net.IOCounters: %w", err)
	}
	if len(stats) == 0 {
		return 0, 0, nil
	}
	return stats[0].BytesRecv, stats[0].BytesSent, nil
}
