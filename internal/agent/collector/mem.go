package collector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

// Memory returns (RAM used %, Swap used %).
//
// When the process is inside a container with cgroup memory limits (LXC,
// Docker --memory, k8s), we read /sys/fs/cgroup directly and compute usage
// the same way Proxmox does (current minus page cache), because gopsutil's
// /proc/meminfo path can leak host-wide stats when lxcfs isn't overlaying.
// Without a real cgroup limit, we fall back to gopsutil.
func Memory(_ context.Context) (ramPct, swapPct float64, err error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, fmt.Errorf("mem.VirtualMemory: %w", err)
	}
	ramPct = v.UsedPercent
	if p, ok := cgroupRAMPct(v.Total); ok {
		ramPct = p
	}

	if s, sErr := mem.SwapMemory(); sErr == nil && s != nil {
		swapPct = s.UsedPercent
	}
	if p, ok := cgroupSwapPct(); ok {
		swapPct = p
	}
	return ramPct, swapPct, nil
}

// cgroupRAMPct returns the container-scoped RAM used %, or false if no real
// memory limit applies. hostTotal is used to detect "unlimited" cgroups
// whose limit is reported as a huge number (cgroup v1) or "max" (v2).
func cgroupRAMPct(hostTotal uint64) (float64, bool) {
	if limit, ok := readUint("/sys/fs/cgroup/memory.max"); ok && limit > 0 && limit < hostTotal {
		current, ok := readUint("/sys/fs/cgroup/memory.current")
		if !ok {
			return 0, false
		}
		cache := readStatKey("/sys/fs/cgroup/memory.stat", "file")
		return pct(safeSub(current, cache), limit), true
	}
	if limit, ok := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok && limit > 0 && limit < hostTotal {
		usage, ok := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes")
		if !ok {
			return 0, false
		}
		cache := readStatKey("/sys/fs/cgroup/memory/memory.stat", "cache")
		return pct(safeSub(usage, cache), limit), true
	}
	return 0, false
}

// MemoryLimitStatus returns a non-empty warning when the agent runs inside a
// cgroup (Docker, k8s, LXC) but no memory limit is configured — in that case
// RAM% will reflect the kernel host's total, which on Docker-in-LXC or
// Docker-in-VM setups is almost never what the operator wants. Empty string
// means either no cgroup or a real limit is set.
func MemoryLimitStatus() string {
	if _, ok := readUint("/sys/fs/cgroup/memory.current"); ok {
		if _, hasLimit := readUint("/sys/fs/cgroup/memory.max"); !hasLimit {
			return "cgroup v2 detected but memory.max=max — RAM% will use kernel host total; set mem_limit (Docker) or lxc.cgroup2.memory.max (LXC) for container-scoped values"
		}
		return ""
	}
	if _, ok := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes"); ok {
		if _, hasLimit := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes"); !hasLimit {
			return "cgroup v1 detected but memory.limit_in_bytes unreadable — RAM% will use kernel host total; set a container memory limit for container-scoped values"
		}
	}
	return ""
}

// cgroupSwapPct returns container-scoped swap used %, or false if no swap
// limit is configured for the cgroup.
func cgroupSwapPct() (float64, bool) {
	if limit, ok := readUint("/sys/fs/cgroup/memory.swap.max"); ok && limit > 0 {
		current, ok := readUint("/sys/fs/cgroup/memory.swap.current")
		if !ok {
			return 0, false
		}
		return pct(current, limit), true
	}
	// cgroup v1: memsw = memory+swap, so swap-only = memsw - memory.
	memswLim, a := readUint("/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes")
	memswUse, b := readUint("/sys/fs/cgroup/memory/memory.memsw.usage_in_bytes")
	memLim, c := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	memUse, d := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if a && b && c && d {
		swapLim := safeSub(memswLim, memLim)
		if swapLim == 0 {
			return 0, false
		}
		return pct(safeSub(memswUse, memUse), swapLim), true
	}
	return 0, false
}

func readUint(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "" || s == "max" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func readStatKey(path, key string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == key {
			if n, err := strconv.ParseUint(f[1], 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

func safeSub(a, b uint64) uint64 {
	if b >= a {
		return 0
	}
	return a - b
}

func pct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}
