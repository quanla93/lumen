// Lumen agent binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_URL              (default "http://localhost:8090")   - hub base URL
//	LUMEN_AGENT_TOKEN          (default "")                        - bearer token (required by hub strict mode)
//	LUMEN_AGENT_INTERVAL       (default "5s")                      - collection interval (Go duration)
//	LUMEN_AGENT_HOST           (default os.Hostname())             - host identifier override
//	LUMEN_AGENT_DISK_PATH      (default "/" or `C:\`)              - filesystem path to report disk% for
//	LUMEN_AGENT_DOCKER_SOCKET  (default "/var/run/docker.sock")    - Docker Engine API socket; missing → container collection silently skipped
//	LUMEN_AGENT_BUFFER_PATH    (default "./lumen-agent-buffer.db") - on-disk overflow queue; survives restarts and hub outages
//	LUMEN_AGENT_BUFFER_MAX_AGE (default "24h")                     - frames older than this are pruned even if unsent
//	LUMEN_AGENT_BUFFER_DRAIN   (default "10")                      - max frames to ship per successful tick (drains backlog gradually)
//	LUMEN_AGENT_CONFIG         (default "/etc/lumen/agent.yaml")   - optional YAML config; fields fill env-var gaps (env always wins)
//
// Every interval, samples CPU%, RAM%, Swap%, Disk%, load averages, network
// throughput, disk I/O, temperature, per-core CPU, and running Docker
// containers, then POSTs the envelope to the hub. If the POST fails the
// envelope is appended to a local bbolt file so it can be replayed when
// the hub comes back.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quanla93/lumen/internal/agent/buffer"
	"github.com/quanla93/lumen/internal/agent/collector"
	"github.com/quanla93/lumen/internal/agent/config"
	"github.com/quanla93/lumen/internal/agent/sender"
	"github.com/quanla93/lumen/internal/shared/api"
	"github.com/quanla93/lumen/internal/shared/envcfg"
)

// defaultConfigPath is the well-known location an Ansible/Salt-style
// deploy can drop a config without flag plumbing. Missing file is a
// quiet no-op so env-only setups keep working.
const defaultConfigPath = "/etc/lumen/agent.yaml"

var version = "dev"

func main() {
	// Stage 1: load YAML config (if present) BEFORE envcfg so its
	// fields are visible to envcfg.String/Duration/etc. The file
	// only fills gaps — anything already in the process environment
	// wins. A malformed file is fatal; missing file is a no-op.
	configPath := os.Getenv("LUMEN_AGENT_CONFIG")
	if configPath == "" {
		configPath = defaultConfigPath
	}
	cfgLoad, cfgErr := config.LoadFile(configPath)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", cfgErr)
		os.Exit(1)
	}

	envcfg.Load()
	hubURL := envcfg.String("LUMEN_HUB_URL", "http://localhost:8090")
	token := envcfg.String("LUMEN_AGENT_TOKEN", "")
	interval := envcfg.Duration("LUMEN_AGENT_INTERVAL", 5*time.Second)
	hostOverride := envcfg.String("LUMEN_AGENT_HOST", "")
	diskPath := envcfg.String("LUMEN_AGENT_DISK_PATH", collector.DefaultDiskPath())
	dockerSocket := envcfg.String("LUMEN_AGENT_DOCKER_SOCKET", collector.DockerSocketPath)
	bufferPath := envcfg.String("LUMEN_AGENT_BUFFER_PATH", "./lumen-agent-buffer.db")
	bufferMaxAge := envcfg.Duration("LUMEN_AGENT_BUFFER_MAX_AGE", 24*time.Hour)
	drainPerTick := envcfg.Int("LUMEN_AGENT_BUFFER_DRAIN", 10)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	host := hostOverride
	if host == "" {
		hn, err := os.Hostname()
		if err != nil {
			logger.Error("hostname lookup failed", "err", err)
			os.Exit(1)
		}
		host = hn
	}

	snd := sender.New(hubURL, token)
	rates := &collector.Rates{}

	// Offline buffer: a bbolt file under bufferPath. If Open fails (e.g.
	// readonly filesystem in a test sandbox) we degrade gracefully — the
	// agent keeps shipping live frames, just without backlog survival.
	buf, err := buffer.Open(buffer.Config{
		Path:   bufferPath,
		MaxAge: bufferMaxAge,
		Logger: logger.With("subsys", "buffer"),
	})
	if err != nil {
		logger.Warn("buffer disabled — could not open on-disk queue",
			"err", err, "path", bufferPath)
		buf = nil
	} else {
		defer buf.Close()
		if n, _ := buf.Size(); n > 0 {
			logger.Info("buffer carries forward frames from previous run",
				"path", bufferPath, "queued", n)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("agent starting",
		"hub", hubURL, "host", host, "interval", interval,
		"disk_path", diskPath, "docker_socket", dockerSocket,
		"buffer_path", bufferPath, "buffer_max_age", bufferMaxAge,
		"buffer_drain_per_tick", drainPerTick)
	if cfgLoad.Path != "" {
		logger.Info("config file loaded",
			"path", cfgLoad.Path,
			"applied_from_yaml", cfgLoad.Applied,
			"skipped_env_wins", cfgLoad.Skipped)
	}

	// Background prune so the on-disk file doesn't grow without bound
	// during long hub outages. Once an hour is enough — pruning is just
	// disk hygiene, not part of the critical path.
	if buf != nil {
		go runPrune(ctx, buf, logger.With("subsys", "buffer"))
	}

	currentInterval := interval
	t := time.NewTimer(currentInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("agent stopped")
			return
		case <-t.C:
			env := collect(ctx, logger, host, diskPath, dockerSocket, rates, version)
			tickOnce(ctx, logger, snd, buf, drainPerTick, env)
			if next := fetchPolicyInterval(ctx, logger, snd, currentInterval); next != currentInterval {
				currentInterval = next
				logger.Info("agent interval updated", "interval", currentInterval)
			}
			t.Reset(currentInterval)
		}
	}
}

func fetchPolicyInterval(ctx context.Context, logger *slog.Logger, snd *sender.Sender, current time.Duration) time.Duration {
	policy, err := snd.FetchPolicy(ctx)
	if err != nil {
		logger.Debug("agent policy fetch failed", "err", err)
		return current
	}
	if policy.CollectionInterval == "" {
		return current
	}
	next, err := time.ParseDuration(policy.CollectionInterval)
	if err != nil || next <= 0 {
		logger.Warn("agent policy interval invalid", "interval", policy.CollectionInterval, "err", err)
		return current
	}
	return next
}

// tickOnce ships the current envelope and, if successful, opportunistically
// drains a few buffered frames so a backlog catches up without a thundering
// herd. The first failure aborts the drain — sending into a hub that's
// already returning errors won't help.
func tickOnce(
	ctx context.Context, logger *slog.Logger,
	snd *sender.Sender, buf *buffer.Buffer, drainMax int,
	env api.IngestRequest,
) {
	if err := snd.Send(ctx, env); err != nil {
		// Hub unreachable / rejected. Try to buffer; if that also
		// fails we log and accept the data loss — there's nowhere
		// else to put it.
		if buf != nil {
			if bufErr := buf.Enqueue(env); bufErr != nil {
				logger.Error("ingest failed AND buffer enqueue failed",
					"send_err", err, "buf_err", bufErr)
			} else {
				size, _ := buf.Size()
				logger.Warn("ingest failed — frame buffered",
					"err", err, "buffer_size", size)
			}
		} else {
			logger.Warn("ingest failed — buffer disabled, frame dropped", "err", err)
		}
		return
	}
	logger.Info("ingested",
		"cpu", env.CpuPct, "ram", env.RamPct, "swap", env.SwapPct,
		"disk", env.DiskPct, "load1", env.Load1,
		"net_rx_kBps", env.NetRxBps/1024, "net_tx_kBps", env.NetTxBps/1024,
		"disk_r_kBps", env.DiskRBps/1024, "disk_w_kBps", env.DiskWBps/1024,
		"temp_c", env.TempC, "cores", len(env.CpuPerCore),
		"containers", len(env.Containers))

	if buf == nil || drainMax <= 0 {
		return
	}
	drained, err := buf.Drain(ctx, drainMax, snd.Send)
	if drained > 0 {
		size, _ := buf.Size()
		logger.Info("buffer drained",
			"shipped", drained, "still_queued", size, "drain_err", err)
	}
}

// runPrune periodically trims expired or over-cap entries.
func runPrune(ctx context.Context, buf *buffer.Buffer, logger *slog.Logger) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	// Eager first pass: a freshly started agent may carry forward an
	// over-long backlog from before a long downtime.
	if n, err := buf.Prune(); err != nil {
		logger.Warn("buffer prune failed", "err", err)
	} else if n > 0 {
		logger.Info("buffer pruned at startup", "deleted", n)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := buf.Prune(); err != nil {
				logger.Warn("buffer prune failed", "err", err)
			} else if n > 0 {
				logger.Info("buffer pruned", "deleted", n)
			}
		}
	}
}

// collect samples every metric the agent reports. Each collector that fails
// is logged at Warn and contributes a zero value so partial data still ships.
func collect(
	ctx context.Context, logger *slog.Logger,
	host, diskPath, dockerSocket string, rates *collector.Rates, agentVersion string,
) api.IngestRequest {
	env := api.IngestRequest{Host: host, Ts: time.Now().UTC()}
	if meta, err := collector.SystemMetadata(ctx, agentVersion); err != nil {
		logger.Debug("system metadata sample partial", "err", err)
		env.System = meta
	} else {
		env.System = meta
	}

	if v, err := collector.CPU(ctx, 500*time.Millisecond); err != nil {
		logger.Warn("cpu sample failed", "err", err)
	} else {
		env.CpuPct = v
	}

	if perCore, err := collector.CPUPerCore(ctx, 200*time.Millisecond); err != nil {
		// Per-core is best-effort; aggregate CPU% above is the primary signal.
		logger.Debug("per-core cpu sample failed", "err", err)
	} else {
		env.CpuPerCore = perCore
	}

	if ram, swap, err := collector.Memory(ctx); err != nil {
		logger.Warn("memory sample failed", "err", err)
	} else {
		env.RamPct = ram
		env.SwapPct = swap
	}

	if v, err := collector.Disk(ctx, diskPath); err != nil {
		logger.Warn("disk sample failed", "err", err, "path", diskPath)
	} else {
		env.DiskPct = v
	}

	if l1, l5, l15, err := collector.Load(ctx); err != nil {
		// Load avg isn't kernel-exposed on Windows — common, don't spam at Warn.
		logger.Debug("load sample unavailable", "err", err)
	} else {
		env.Load1 = l1
		env.Load5 = l5
		env.Load15 = l15
	}

	if s, err := rates.Sample(ctx, env.Ts); err != nil {
		logger.Debug("net/disk rate sample partial", "err", err)
		// s may still be partially populated — fall through and use it.
		env.NetRxBps, env.NetTxBps = s.NetRxBps, s.NetTxBps
		env.DiskRBps, env.DiskWBps = s.DiskRBps, s.DiskWBps
	} else {
		env.NetRxBps, env.NetTxBps = s.NetRxBps, s.NetTxBps
		env.DiskRBps, env.DiskWBps = s.DiskRBps, s.DiskWBps
	}

	if v, err := collector.Temperature(ctx); err != nil {
		logger.Debug("temperature sample failed", "err", err)
	} else {
		env.TempC = v
	}

	cs, reason, err := collector.DockerContainers(ctx, dockerSocket)
	env.Containers = cs
	switch reason {
	case collector.DockerSocketMissing:
		// host has no Docker at all → quiet by design
	case collector.DockerSocketRefused:
		// First failure surfaces at Warn so the operator sees it once
		// (and learns about Docker Desktop's socket toggle); thereafter
		// the same error path returns no wrapped message and stays Debug.
		if err != nil {
			logger.Warn("docker containers unavailable — on macOS Docker Desktop, enable Settings → Advanced → \"Allow the default Docker socket to be used\"",
				"err", err, "socket", dockerSocket)
		} else {
			logger.Debug("docker containers sample failed (already warned once)")
		}
	default:
		if err != nil {
			logger.Debug("docker containers sample partial", "err", err)
		}
	}

	return env
}
