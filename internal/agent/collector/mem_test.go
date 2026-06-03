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

