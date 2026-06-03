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
	// gopsutil's UsedPercent counts SReclaimable as cache; lxcfs and other
	// /proc/meminfo providers report it large enough that gopsutil's number
	// reads "near zero used" while Proxmox / `free -m` show a normal usage
	// level. Prefer the MemAvailable-based formula when the kernel exposes
	// MemAvailable (Linux 3.14+) so RAM% lines up with what operators see
	// in their hypervisor UI.
	if v.Available > 0 && v.Total > 0 {
		ramPct = float64(v.Total-v.Available) / float64(v.Total) * 100
	}
	// Skip cgroup override when /proc/meminfo is bind-mounted from the host:
	// gopsutil's view above is already container-scoped (lxcfs), and the
	// Docker container's own cgroup would otherwise leak through here showing
	// just the agent process's memory (~5 MB / 4 GB ≈ 0.1%). The cgroup path
	// is still useful when the operator chose mem_limit-only (no bind-mount)
	// because they're intentionally monitoring the container's own footprint.
	if !procMeminfoIsBindMounted() {
		if p, ok := cgroupRAMPct(v.Total); ok {
			ramPct = p
		}
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

// MemoryDiagnostics is a one-shot startup probe: it returns the agent's view
// of the moving parts that decide which RAM% path wins (gopsutil vs cgroup),
// so an operator can grep startup logs to confirm the bind-mount or cgroup
// override is doing what they expect. Quiet on bare-host setups; useful on
// Docker-in-LXC where Lumen has shipped four 0.6.5.x patches debugging this.
func MemoryDiagnostics() string {
	bind := procMeminfoIsBindMounted()
	miSize := -1
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		miSize = len(data)
	}
	memSample := ""
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.SplitN(string(data), "\n", 3)
		if len(lines) > 0 {
			memSample = lines[0]
		}
	}
	cgV2Max, _ := readUint("/sys/fs/cgroup/memory.max")
	cgV2Cur, _ := readUint("/sys/fs/cgroup/memory.current")
	cgV1Max, _ := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	cgV1Cur, _ := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	return fmt.Sprintf(
		"meminfo_bindmount=%v mountinfo_bytes=%d meminfo_first_line=%q "+
			"cgv2_max=%d cgv2_cur=%d cgv1_max=%d cgv1_cur=%d",
		bind, miSize, memSample, cgV2Max, cgV2Cur, cgV1Max, cgV1Cur,
	)
}

// MemoryLimitStatus returns a non-empty warning when the agent runs inside a
// cgroup (Docker, k8s, LXC) without either a memory limit OR a bind-mounted
// /proc/meminfo to give it the right view. Either of those is enough to get
// container-scoped RAM%; with neither, the agent reports the kernel host's
// total, which on Docker-in-LXC/VM setups is almost never what the operator
// wants. Empty string means the setup is already correct.
func MemoryLimitStatus() string {
	inCgroupV2 := false
	if _, ok := readUint("/sys/fs/cgroup/memory.current"); ok {
		inCgroupV2 = true
	}
	inCgroupV1 := false
	if !inCgroupV2 {
		if _, ok := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes"); ok {
			inCgroupV1 = true
		}
	}
	if !inCgroupV2 && !inCgroupV1 {
		return ""
	}

	if procMeminfoIsBindMounted() {
		return ""
	}
	if inCgroupV2 {
		if _, hasLimit := readUint("/sys/fs/cgroup/memory.max"); hasLimit {
			return ""
		}
		return "RAM% will use the kernel host's total — bind-mount /proc/meminfo:/proc/meminfo:ro from the host (preferred for Docker-in-LXC), or set mem_limit (Docker) / lxc.cgroup2.memory.max (LXC)"
	}
	if _, hasLimit := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes"); hasLimit {
		return ""
	}
	return "RAM% will use the kernel host's total — bind-mount /proc/meminfo:/proc/meminfo:ro from the host, or set a container memory limit"
}

// procMeminfoIsBindMounted returns true when /proc/meminfo appears as its own
// mount entry in /proc/self/mountinfo — the signature of a `-v /proc/meminfo:...`
// bind mount (Docker), an lxcfs FUSE bind (LXC native), or similar. Used to
// surface the host's lxcfs-overlaid view into a Docker container running
// inside an LXC.
//
// We check mountinfo, not /proc/mounts: Docker's file-level bind mounts only
// appear in mountinfo. mountinfo line format is documented at
// https://www.kernel.org/doc/Documentation/filesystems/proc.txt; the 5th
// field (0-indexed 4) is the mount point.
func procMeminfoIsBindMounted() bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	return parseMountinfoForMeminfo(data)
}

// parseMountinfoForMeminfo is the pure parser, split out so a unit test can
// feed it the exact byte sequence a real kernel produced. The integration
// concern (os.ReadFile of a procfs pseudo-file) is in procMeminfoIsBindMounted.
func parseMountinfoForMeminfo(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[4] == "/proc/meminfo" {
			return true
		}
	}
	return false
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
