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
// The deployment shape that drove every v0.6.5.x patch is Docker-in-LXC:
// the agent runs in a Docker container running inside a Proxmox LXC.
// What the operator wants to see is the LXC's RAM% — exactly what Proxmox
// shows. Three views are available, in priority order:
//
//  1. **/host-cgroup bind-mount** — the operator mounts the LXC's
//     /sys/fs/cgroup into the agent container as /host-cgroup:ro and we
//     read memory.current / memory.max directly. This is the only path
//     that works for Docker-in-LXC, because lxcfs serves /proc/meminfo
//     according to the *caller's* cgroup; from inside Docker the caller's
//     cgroup is Docker's, and lxcfs returns a near-empty view (~0.18%).
//
//  2. **/proc/meminfo bind-mount via lxcfs** — works on native LXC (no
//     Docker), where the agent is in the LXC's cgroup so lxcfs's
//     /proc/meminfo numbers ARE the LXC view. gopsutil is bypassed
//     because gopsutil v4 mixes /sys/fs/cgroup/memory.current into
//     Available and corrupts the lxcfs numbers.
//
//  3. **gopsutil + cgroup override** — bare-host Docker with `mem_limit:`,
//     or no container at all. Existing behaviour.
func Memory(_ context.Context) (ramPct, swapPct float64, err error) {
	v, vErr := mem.VirtualMemory()
	if vErr != nil {
		return 0, 0, fmt.Errorf("mem.VirtualMemory: %w", vErr)
	}
	switch {
	case hostCgroupAvailable():
		if p, ok := hostCgroupRAMPct(v.Total); ok {
			ramPct = p
		}
	case procMeminfoIsBindMounted():
		if p, ok := procMeminfoRamPct(); ok {
			ramPct = p
		}
	default:
		ramPct = v.UsedPercent
		if v.Available > 0 && v.Total > 0 {
			ramPct = float64(v.Total-v.Available) / float64(v.Total) * 100
		}
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

// hostCgroupAvailable returns true when the operator bind-mounted the
// containing cgroup tree (LXC's /sys/fs/cgroup, in the Docker-in-LXC case)
// to /host-cgroup. Detection is "does a known memory file exist there",
// cheap and unambiguous.
func hostCgroupAvailable() bool {
	if _, err := os.Stat("/host-cgroup/memory.current"); err == nil {
		return true
	}
	if _, err := os.Stat("/host-cgroup/memory/memory.usage_in_bytes"); err == nil {
		return true
	}
	return false
}

// hostCgroupRAMPct reads the bind-mounted LXC cgroup view (preferred) or
// returns false. Same accounting style as cgroupRAMPct: subtract page cache
// from current so the number matches what Proxmox displays.
func hostCgroupRAMPct(hostTotal uint64) (float64, bool) {
	if limit, ok := readUint("/host-cgroup/memory.max"); ok && limit > 0 && limit < hostTotal {
		current, ok := readUint("/host-cgroup/memory.current")
		if !ok {
			return 0, false
		}
		cache := readStatKey("/host-cgroup/memory.stat", "file")
		return pct(safeSub(current, cache), limit), true
	}
	if limit, ok := readUint("/host-cgroup/memory/memory.limit_in_bytes"); ok && limit > 0 && limit < hostTotal {
		usage, ok := readUint("/host-cgroup/memory/memory.usage_in_bytes")
		if !ok {
			return 0, false
		}
		cache := readStatKey("/host-cgroup/memory/memory.stat", "cache")
		return pct(safeSub(usage, cache), limit), true
	}
	return 0, false
}

// procMeminfoRamPct reads /proc/meminfo (the bind-mounted lxcfs view on
// Docker-in-LXC) and returns (MemTotal - MemAvailable) / MemTotal. False on
// parse failure or implausible values so the caller can decide what to do.
func procMeminfoRamPct() (float64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	return parseMeminfoRamPct(data)
}

// parseMeminfoRamPct is the pure parser, split out for testability with the
// exact kernel/lxcfs byte sequence captured in the field.
func parseMeminfoRamPct(data []byte) (float64, bool) {
	var total, available uint64
	var haveTotal, haveAvail bool
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, perr := strconv.ParseUint(f[1], 10, 64)
		if perr != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total = n
			haveTotal = true
		case "MemAvailable:":
			available = n
			haveAvail = true
		}
	}
	if !haveTotal || !haveAvail || total == 0 || available > total {
		return 0, false
	}
	return float64(total-available) / float64(total) * 100, true
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
		// First 3 KEY lines (Total/Free/Available) is enough to verify whether
		// lxcfs is giving us the LXC view or the Docker-cgroup-scoped view.
		var want = []string{"MemTotal:", "MemFree:", "MemAvailable:"}
		var picked []string
		for _, line := range strings.Split(string(data), "\n") {
			for _, w := range want {
				if strings.HasPrefix(line, w) {
					picked = append(picked, strings.Join(strings.Fields(line), " "))
				}
			}
			if len(picked) == len(want) {
				break
			}
		}
		memSample = strings.Join(picked, "; ")
	}
	cgV2Max, _ := readUint("/sys/fs/cgroup/memory.max")
	cgV2Cur, _ := readUint("/sys/fs/cgroup/memory.current")
	cgV1Max, _ := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	cgV1Cur, _ := readUint("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	hcAvail := hostCgroupAvailable()
	hcV2Max, _ := readUint("/host-cgroup/memory.max")
	hcV2Cur, _ := readUint("/host-cgroup/memory.current")
	hcV1Max, _ := readUint("/host-cgroup/memory/memory.limit_in_bytes")
	hcV1Cur, _ := readUint("/host-cgroup/memory/memory.usage_in_bytes")
	return fmt.Sprintf(
		"meminfo_bindmount=%v mountinfo_bytes=%d meminfo=%q "+
			"cgv2_max=%d cgv2_cur=%d cgv1_max=%d cgv1_cur=%d "+
			"host_cgroup_mounted=%v hcv2_max=%d hcv2_cur=%d hcv1_max=%d hcv1_cur=%d",
		bind, miSize, memSample, cgV2Max, cgV2Cur, cgV1Max, cgV1Cur,
		hcAvail, hcV2Max, hcV2Cur, hcV1Max, hcV1Cur,
	)
}

// MemoryLimitStatus returns a non-empty warning when the agent runs inside a
// cgroup (Docker, k8s, LXC) without any of the signals it needs to report a
// useful RAM%. The signals, in preference order:
//
//  1. /host-cgroup bind-mount — only path that works for Docker-in-LXC.
//  2. /proc/meminfo bind-mount via lxcfs — works for native LXC.
//  3. A real cgroup memory limit on the agent's own cgroup — works for
//     bare-host Docker with `mem_limit:`.
//
// With none of those, the agent reports the kernel host's total, which is
// almost never what the operator wants. Empty string means the setup is
// already correct.
func MemoryLimitStatus() string {
	if hostCgroupAvailable() {
		return ""
	}
	if procMeminfoIsBindMounted() {
		return ""
	}

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

	if inCgroupV2 {
		if _, hasLimit := readUint("/sys/fs/cgroup/memory.max"); hasLimit {
			return ""
		}
		return "RAM% will use the kernel host's total — for Docker-in-LXC bind-mount /sys/fs/cgroup:/host-cgroup:ro from the LXC (matches Proxmox UI); for bare-host Docker set mem_limit; for native LXC bind-mount /proc/meminfo:/proc/meminfo:ro"
	}
	if _, hasLimit := readUint("/sys/fs/cgroup/memory/memory.limit_in_bytes"); hasLimit {
		return ""
	}
	return "RAM% will use the kernel host's total — bind-mount /sys/fs/cgroup:/host-cgroup:ro from the host (Docker-in-LXC), bind-mount /proc/meminfo:/proc/meminfo:ro (native LXC), or set a container memory limit"
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
