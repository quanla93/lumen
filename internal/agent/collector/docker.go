package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quanla93/lumen/internal/shared/api"
)

// DockerSocketPath is the conventional path to the Docker Engine API socket.
// Override via the LUMEN_AGENT_DOCKER_SOCKET env var in main.
const DockerSocketPath = "/var/run/docker.sock"

// firstSockErr is set to non-zero once we've reported the first
// connection-refused / lookup error for the Docker socket, so we don't
// spam the same message every interval after the operator already knows.
var firstSockErr atomic.Bool

// DockerErrReason distinguishes a "this host has no Docker at all" non-event
// from a "Docker is there but I can't reach it" actionable error. Callers
// can switch on it to decide between Debug and Warn-once logging.
type DockerErrReason int

const (
	DockerOK            DockerErrReason = iota
	DockerSocketMissing                 // os.Stat failed → likely Docker not installed / socket not mounted
	DockerSocketRefused                 // socket exists but connection refused (Docker Desktop disallow, daemon down)
)

// DockerContainers returns one ContainerInfo per running container the
// local Docker daemon reports. If the socket isn't readable (no Docker on
// this host, missing bind mount in the agent container, no permission)
// it returns (nil, DockerSocketMissing, nil) — agents that don't run on a
// Docker host simply report no containers, no warnings spammed every tick.
//
// We intentionally do NOT pull in github.com/docker/docker — that SDK
// brings ~200 transitive deps and a 30+ MB binary bloat. We only need
// two endpoints; a tiny stdlib HTTP-over-unix-socket client is enough.
func DockerContainers(ctx context.Context, socketPath string) ([]api.ContainerInfo, DockerErrReason, error) {
	if socketPath == "" {
		socketPath = DockerSocketPath
	}
	// Cheap probe: stat the socket. Missing socket → "no Docker here",
	// not an error worth surfacing each tick.
	if _, err := os.Stat(socketPath); err != nil {
		return nil, DockerSocketMissing, nil
	}

	c := newDockerClient(socketPath)
	list, err := c.listContainers(ctx)
	if err != nil {
		// Connection refused inside a container with the socket bind-mounted
		// usually means Docker Desktop on macOS hasn't enabled
		// "Allow the default Docker socket to be used" in Settings →
		// Advanced. Surface the wrapped error ONCE so the operator can
		// act; later ticks return DockerSocketRefused with err=nil so the
		// caller stays silent.
		if !firstSockErr.Swap(true) {
			return nil, DockerSocketRefused, fmt.Errorf("docker list: %w", err)
		}
		return nil, DockerSocketRefused, nil
	}
	if len(list) == 0 {
		return nil, DockerOK, nil
	}

	// Fan out the per-container stats fetch with bounded concurrency so
	// 50+ containers don't all hammer the socket simultaneously.
	const maxParallel = 5
	sem := make(chan struct{}, maxParallel)
	out := make([]api.ContainerInfo, len(list))
	var wg sync.WaitGroup
	for i, item := range list {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, item dockerListItem) {
			defer wg.Done()
			defer func() { <-sem }()

			info := api.ContainerInfo{
				ID:    shortID(item.Id),
				Name:  cleanName(item.Names),
				Image: item.Image,
				State: item.State,
			}
			// Only running containers expose useful stats; for stopped /
			// paused ones, ship the metadata only.
			if item.State == "running" {
				if s, err := c.stats(ctx, item.Id); err == nil {
					info.CpuPct = computeContainerCpuPct(s)
					info.MemUsedBytes = computeContainerMemUsed(s)
					info.MemLimitBytes = s.MemoryStats.Limit
					if info.MemLimitBytes > 0 {
						info.MemPct = float64(info.MemUsedBytes) / float64(info.MemLimitBytes) * 100
					}
				}
			}
			out[i] = info
		}(i, item)
	}
	wg.Wait()
	return out, DockerOK, nil
}

// ─── minimal Docker Engine API client over a unix socket ─────────────────────

type dockerClient struct {
	http *http.Client
}

func newDockerClient(socketPath string) *dockerClient {
	return &dockerClient{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: 1 * time.Second}).DialContext(ctx, "unix", socketPath)
				},
				MaxIdleConns:       4,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: true,
			},
			// 5s envelope is generous: list is fast, but /stats?stream=false
			// blocks for ~1s while the daemon collects a delta sample.
			Timeout: 5 * time.Second,
		},
	}
}

func (c *dockerClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("docker api %s: %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

type dockerListItem struct {
	Id    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	State string   `json:"State"`
}

type dockerStats struct {
	CpuStats struct {
		CpuUsage       struct{ TotalUsage uint64 } `json:"cpu_usage"`
		SystemCpuUsage uint64                      `json:"system_cpu_usage"`
		OnlineCpus     uint64                      `json:"online_cpus"`
	} `json:"cpu_stats"`
	PrecpuStats struct {
		CpuUsage       struct{ TotalUsage uint64 } `json:"cpu_usage"`
		SystemCpuUsage uint64                      `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
}

func (c *dockerClient) listContainers(ctx context.Context) ([]dockerListItem, error) {
	var out []dockerListItem
	if err := c.get(ctx, "/containers/json?all=false", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *dockerClient) stats(ctx context.Context, id string) (*dockerStats, error) {
	var out dockerStats
	if err := c.get(ctx, "/containers/"+id+"/stats?stream=false", &out); err != nil {
		return nil, err
	}
	if out.CpuStats.SystemCpuUsage == 0 {
		// stream=false on a just-started container can return an all-zero
		// frame; treat it as "no data this tick" so the UI doesn't show
		// a phantom 0% with stale denominator.
		return nil, errors.New("empty stats frame")
	}
	return &out, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func cleanName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// computeContainerCpuPct converts the Docker stats delta into a 0-100%
// value normalized to the whole host (so 100% = all online CPUs saturated
// by this container — matches `docker stats` output).
func computeContainerCpuPct(s *dockerStats) float64 {
	cpuDelta := float64(s.CpuStats.CpuUsage.TotalUsage) - float64(s.PrecpuStats.CpuUsage.TotalUsage)
	sysDelta := float64(s.CpuStats.SystemCpuUsage) - float64(s.PrecpuStats.SystemCpuUsage)
	if sysDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	online := float64(s.CpuStats.OnlineCpus)
	if online == 0 {
		online = 1
	}
	pct := (cpuDelta / sysDelta) * online * 100
	if pct < 0 {
		return 0
	}
	if pct > 100*online {
		return 100 * online // hard cap; container can't use more than all cores
	}
	return pct
}

// computeContainerMemUsed subtracts the page cache so we report "real"
// process-attributable memory, matching `docker stats` and what an
// operator expects to see.
func computeContainerMemUsed(s *dockerStats) uint64 {
	if s.MemoryStats.Usage > s.MemoryStats.Stats.InactiveFile {
		return s.MemoryStats.Usage - s.MemoryStats.Stats.InactiveFile
	}
	return s.MemoryStats.Usage
}
