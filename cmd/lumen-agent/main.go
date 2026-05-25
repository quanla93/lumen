// Lumen agent binary.
//
// Configuration is read from environment variables (12-factor). A .env file
// in the CWD is loaded automatically if present (dev convenience).
//
//	LUMEN_HUB_URL          (default "http://localhost:8090")  - hub base URL
//	LUMEN_AGENT_TOKEN      (default "")                       - bearer token (ignored by hub until Phase 2)
//	LUMEN_AGENT_INTERVAL   (default "5s")                     - collection interval (Go duration)
//	LUMEN_AGENT_HOST       (default os.Hostname())            - host identifier override
//
// Every interval, samples host CPU% via gopsutil and POSTs an ingest envelope
// to the hub. Phase 2 adds the rest of the collector matrix and a local
// BoltDB ring buffer for offline resilience.
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("agent starting", "hub", hubURL, "host", host, "interval", interval)

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("agent stopped")
			return
		case <-t.C:
			cpuPct, err := collector.CPU(ctx, 500*time.Millisecond)
			if err != nil {
				logger.Warn("cpu sample failed", "err", err)
				continue
			}
			env := api.IngestRequest{Host: host, Ts: time.Now().UTC(), CpuPct: cpuPct}
			if err := snd.Send(ctx, env); err != nil {
				logger.Warn("ingest send failed", "err", err)
				continue
			}
			logger.Info("ingested", "cpu_pct", cpuPct)
		}
	}
}
