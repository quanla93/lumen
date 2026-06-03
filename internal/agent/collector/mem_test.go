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
