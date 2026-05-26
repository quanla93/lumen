// Lumen agent binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_URL          (default "http://localhost:8090")  - hub base URL
//	LUMEN_AGENT_TOKEN      (default "")                       - bearer token (ignored by hub until Phase 2 auth)
//	LUMEN_AGENT_INTERVAL   (default "5s")                     - collection interval (Go duration)
//	LUMEN_AGENT_HOST       (default os.Hostname())            - host identifier override
//	LUMEN_AGENT_DISK_PATH  (default "/" or `C:\`)             - filesystem path to report disk% for
//
// Every interval, samples CPU%, RAM%, Swap%, Disk%, and load averages via
// gopsutil and POSTs an envelope to the hub. Phase 2 adds disk I/O, network
// throughput, temperatures, per-container metrics, and a local offline buffer.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lumenhq/lumen/internal/agent/collector"
	"github.com/lumenhq/lumen/internal/agent/sender"
	"github.com/lumenhq/lumen/internal/shared/api"
	"github.com/lumenhq/lumen/internal/shared/envcfg"
)

func main() {
	envcfg.Load()
	hubURL := envcfg.String("LUMEN_HUB_URL", "http://localhost:8090")
	token := envcfg.String("LUMEN_AGENT_TOKEN", "")
	interval := envcfg.Duration("LUMEN_AGENT_INTERVAL", 5*time.Second)
	hostOverride := envcfg.String("LUMEN_AGENT_HOST", "")
	diskPath := envcfg.String("LUMEN_AGENT_DISK_PATH", collector.DefaultDiskPath())

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("agent starting",
		"hub", hubURL, "host", host, "interval", interval, "disk_path", diskPath)

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("agent stopped")
			return
		case <-t.C:
			env := collect(ctx, logger, host, diskPath, rates)
			if err := snd.Send(ctx, env); err != nil {
				logger.Warn("ingest send failed", "err", err)
				continue
			}
			logger.Info("ingested",
				"cpu", env.CpuPct, "ram", env.RamPct, "swap", env.SwapPct,
				"disk", env.DiskPct, "load1", env.Load1,
				"net_rx_kBps", env.NetRxBps/1024, "net_tx_kBps", env.NetTxBps/1024,
				"disk_r_kBps", env.DiskRBps/1024, "disk_w_kBps", env.DiskWBps/1024,
				"temp_c", env.TempC, "cores", len(env.CpuPerCore))
		}
	}
}

// collect samples every metric the agent reports. Each collector that fails
// is logged at Warn and contributes a zero value so partial data still ships.
func collect(
	ctx context.Context, logger *slog.Logger,
	host, diskPath string, rates *collector.Rates,
) api.IngestRequest {
	env := api.IngestRequest{Host: host, Ts: time.Now().UTC()}

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

	return env
}
