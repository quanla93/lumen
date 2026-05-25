package collector

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/mem"
)

// Memory returns (RAM used %, Swap used %). Swap is best-effort: if the
// host has no swap or the lookup fails, swapPct is 0 and the error is nil.
func Memory(_ context.Context) (ramPct, swapPct float64, err error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, fmt.Errorf("mem.VirtualMemory: %w", err)
	}
	if s, sErr := mem.SwapMemory(); sErr == nil && s != nil {
		swapPct = s.UsedPercent
	}
	return v.UsedPercent, swapPct, nil
}
