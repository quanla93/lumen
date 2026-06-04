package collector

import "testing"

// Real /proc/self/mountinfo entries captured from a Docker-in-LXC agent
// where the user reported the bind-mount detection was silently failing.
// Each test exercises the kernel format the agent will actually see.
func TestParseMountinfoForMeminfo(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "docker-in-lxc with lxcfs fuse bind-mount on meminfo",
			data: `2354 1387 0:165 / / rw,relatime - overlay overlay rw,lowerdir=/x
2356 2354 0:230 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
2391 2354 0:250 / /sys ro,nosuid,nodev,noexec,relatime - sysfs sysfs ro
2396 2356 0:132 /proc/cpuinfo /proc/cpuinfo ro,nosuid,nodev,relatime - fuse.lxcfs lxcfs rw,user_id=0,group_id=0,allow_other
2397 2356 0:132 /proc/meminfo /proc/meminfo ro,nosuid,nodev,relatime - fuse.lxcfs lxcfs rw,user_id=0,group_id=0,allow_other
`,
			want: true,
		},
		{
			name: "no bind-mount on meminfo",
			data: `2354 1387 0:165 / / rw,relatime - overlay overlay rw
2356 2354 0:230 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
`,
			want: false,
		},
		{
			name: "meminfo as source path but mount point elsewhere",
			data: `1 2 0:1 /proc/meminfo /tmp/foo rw - tmpfs tmpfs rw
`,
			want: false,
		},
		{
			name: "empty",
			data: ``,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMountinfoForMeminfo([]byte(tc.data))
			if got != tc.want {
				t.Fatalf("parseMountinfoForMeminfo: got %v want %v", got, tc.want)
			}
		})
	}
}

// Real /proc/meminfo first 10 lines captured from the same Docker-in-LXC
// agent where v0.6.5.4–v0.6.5.6 reported RAM% ≈ 0.06% while the operator
// expected ~5%. The hand-computed expected value is (4194304 - 3956832) /
// 4194304 * 100 ≈ 5.66% — what Proxmox UI shows. gopsutil v4 returned
// ~0.05% on the same file because it overrode Available with the Docker
// container's cgroup memory.current (a few MB).
func TestParseMeminfoRamPct(t *testing.T) {
	const realLxcfsView = `MemTotal:        4194304 kB
MemFree:         3335356 kB
MemAvailable:    3956832 kB
Buffers:               0 kB
Cached:           621776 kB
SwapCached:            0 kB
Active:           318384 kB
Inactive:         502152 kB
Active(anon):     142480 kB
Inactive(anon):    56580 kB
`
	pct, ok := parseMeminfoRamPct([]byte(realLxcfsView))
	if !ok {
		t.Fatal("parseMeminfoRamPct: ok=false on a well-formed /proc/meminfo")
	}
	const want = 5.66
	if diff := pct - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("parseMeminfoRamPct: got %.4f, want ≈ %.2f (±0.01)", pct, want)
	}

	cases := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "missing MemAvailable",
			data: "MemTotal: 4194304 kB\n",
			want: false,
		},
		{
			name: "zero total",
			data: "MemTotal: 0 kB\nMemAvailable: 100 kB\n",
			want: false,
		},
		{
			name: "available larger than total (corrupted/transient)",
			data: "MemTotal: 100 kB\nMemAvailable: 999 kB\n",
			want: false,
		},
		{
			name: "blank",
			data: "",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseMeminfoRamPct([]byte(tc.data))
			if ok != tc.want {
				t.Fatalf("ok=%v, want %v", ok, tc.want)
			}
		})
	}
}

// computeCgroupRAMPct must answer the Docker-in-LXC reproducer correctly:
// LXC has a 4 GiB limit set by Proxmox on the parent cgroup, but from inside
// the LXC's namespace memory.max reports "max" (0). memory.current = 688 MB
// (LXC's actual usage, accumulated across all child cgroups). memory.stat's
// "file" line = ~500 MB of page cache. Expected: (688 - 500) / 4096 ≈ 4.6%,
// matching what Proxmox UI displays.
func TestComputeCgroupRAMPct(t *testing.T) {
	const (
		MB = uint64(1024 * 1024)
		GB = uint64(1024 * 1024 * 1024)
	)
	tests := []struct {
		name                            string
		current, limit, cache, hostTot  uint64
		wantPct                         float64
	}{
		{
			name:    "docker-in-lxc: limit=max, use hostTotal",
			current: 688 * MB,
			limit:   0,
			cache:   500 * MB,
			hostTot: 4 * GB,
			wantPct: 4.59, // (188 MB / 4096 MB) * 100
		},
		{
			name:    "docker mem_limit set, no cache",
			current: 50 * MB,
			limit:   512 * MB,
			cache:   0,
			hostTot: 16 * GB,
			wantPct: 9.77, // (50 / 512) * 100
		},
		{
			name:    "limit > hostTotal (cgroup v1 sentinel) falls back",
			current: 200 * MB,
			limit:   1 << 62, // way larger than any real config
			cache:   0,
			hostTot: 4 * GB,
			wantPct: 4.88, // (200 / 4096) * 100
		},
		{
			name:    "everything zero → 0",
			current: 0,
			limit:   0,
			cache:   0,
			hostTot: 0,
			wantPct: 0,
		},
		{
			name:    "cache exceeds current → 0 (clamped by safeSub)",
			current: 100 * MB,
			limit:   1 * GB,
			cache:   200 * MB,
			hostTot: 4 * GB,
			wantPct: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeCgroupRAMPct(tc.current, tc.limit, tc.cache, tc.hostTot)
			if diff := got - tc.wantPct; diff > 0.05 || diff < -0.05 {
				t.Fatalf("got %.4f, want ≈ %.2f (±0.05)", got, tc.wantPct)
			}
		})
	}
}

