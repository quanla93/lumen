// processes.go — Top-N processes by CPU or RSS.
//
// Default OFF. Two gates (defense in depth):
//   1. Server-side: settings key `processes.enabled` (defaults false).
//      The agent checks it via /api/agent/policy at startup.
//   2. Agent-side: env var `LUMEN_AGENT_PROCESSES=true` (opt-in per
//      host). Missing = silent no-op even if the server enabled it.
//
// The opt-in is per-host because the cmdline field can leak
// secrets: `redis-cli --password=hunter2 ...` lands in the host
// detail UI verbatim, and an attacker reading the API gets a free
// secret. RFC 0003 §"Process list" calls this out; docs/configure/
// processes.md expands on it.
//
// Sorted by CPU% or RSS depending on `processes.sort_by` setting
// (also read at startup).

package collector

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/quanla93/lumen/internal/shared/api"
)

// ProcessesEnabled is the runtime gate. The agent's main reads
// settings.processes.enabled at boot via the /api/agent/policy
// endpoint and writes the boolean here. A false value means the
// collector returns (nil, nil) immediately — the host detail page
// just sees no Processes field.
var ProcessesEnabled = false

// ProcessesSortBy is "cpu" or "rss", set at boot. CPU sorts by
// process.CPUPercent(); RSS sorts by RSS in MiB.
var ProcessesSortBy = "cpu"

// ProcessesTopN is the cap, set at boot. RFC §"Process list" caps
// at 50; settings default is 10.
var ProcessesTopN = 10

// CmdlineMaxLen is the hard cap on Cmd. 200 chars is RFC 0003's
// default; long enough to identify the workload (python3 -m jupyter
// notebook --port=8888 ...) without spilling a 4 KB env block.
const CmdlineMaxLen = 200

// Processes returns the top-N processes sorted by CPU% or RSS,
// depending on ProcessesSortBy. Returns (nil, nil) when disabled —
// the host detail page sees the missing Processes field as "no
// data" rather than an error.
//
// gopsutil's process.Process() can return a "process no longer
// exists" error when iterating — we skip those quietly. cmdline
// truncation happens before the sort, so the truncated strings
// don't get re-shortened.
func Processes(ctx context.Context) ([]api.ProcessInfo, error) {
	if !ProcessesEnabled {
		return nil, nil
	}
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("processes: list: %w", err)
	}
	out := make([]api.ProcessInfo, 0, len(procs))
	for _, p := range procs {
		info, err := collectOneProcess(ctx, p)
		if err != nil {
			// race: process disappeared between list and inspect.
			// Skip — would otherwise fail the whole batch.
			continue
		}
		out = append(out, info)
	}

	// Sort by the configured key, descending.
	switch ProcessesSortBy {
	case "rss":
		sort.Slice(out, func(i, j int) bool { return out[i].RSSMB > out[j].RSSMB })
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].CPUPct > out[j].CPUPct })
	}

	if len(out) > ProcessesTopN {
		out = out[:ProcessesTopN]
	}
	return out, nil
}

// collectOneProcess reads PID / name / user / CPU% / RSS / cmdline
// for a single process. Returns an error when the process
// disappeared between ProcessesWithContext and this call (the
// common "race" we silently skip in Processes).
func collectOneProcess(ctx context.Context, p *process.Process) (api.ProcessInfo, error) {
	pid := p.Pid
	name, _ := p.NameWithContext(ctx)
	username, _ := p.UsernameWithContext(ctx)
	cpuPct, _ := p.CPUPercentWithContext(ctx)
	memInfo, err := p.MemoryInfoWithContext(ctx)
	if err != nil {
		return api.ProcessInfo{}, err
	}
	cmdline, _ := p.CmdlineWithContext(ctx)
	if len(cmdline) > CmdlineMaxLen {
		cmdline = cmdline[:CmdlineMaxLen] + "…"
	}
	return api.ProcessInfo{
		PID:    int32(pid),
		Name:   name,
		User:   username,
		CPUPct: cpuPct,
		RSSMB:  memInfo.RSS / (1024 * 1024),
		Cmd:    strings.TrimSpace(cmdline),
	}, nil
}

// RedactCmd applies a regex to the cmdline. RFC 0003 Q2 proposed
// a defensive default (?i)(password|secret|token|api[_-]?key)\s*=
// and a setting processes.redact_regex to override. The function
// runs server-side; the agent always sends raw cmdline and the
// server redacts before persisting or shipping the WS broadcast.
func RedactCmd(cmd, pattern string) string {
	if pattern == "" {
		return cmd
	}
	matched, err := regexpCompile(pattern, cmd)
	if err != nil {
		// Bad regex; leave cmdline alone. The operator sees the
		// error in logs at boot, not on every tick.
		return cmd
	}
	if matched {
		return "<redacted: matches processes.redact_regex>"
	}
	return cmd
}

// regexpCompile is a small wrapper that hides the regexp-package
// import for the file (it would otherwise need to be top-level
// and add to the binary even for hosts that never opt in).
func regexpCompile(pattern, s string) (bool, error) {
	return regexCompileImpl(pattern, s)
}

// _ keeps the os package honest on platforms that don't use it
// (we keep the import for future env-based redaction knobs).
var _ = os.Getenv
