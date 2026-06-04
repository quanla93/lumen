# RFC 0003 — Beszel bundle 1 (GPU + processes + maintenance)

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 3
- **Effort**: 5 days (GPU 2d + processes 1.5d + maintenance 1.5d)

## Motivation

Three Beszel features lumen-comparable users miss most:

- **GPU monitoring**: homelabs increasingly run Plex/Jellyfin transcoding + local LLM inference, both GPU-bound.
- **Process list**: "what's eating my RAM" is the single most common ad-hoc diagnostic.
- **Maintenance windows**: planned reboots / firmware updates / cable swaps shouldn't page anybody.

Bundling them keeps the sprint cohesive (all three extend existing collector / engine surfaces) and lets us ship a meaningful chunk of Beszel parity in one release.

## Scope

### GPU monitoring
**In**: NVIDIA (`nvidia-smi`) + AMD (`rocm-smi`) per-GPU utilization, memory, temperature. Multi-GPU per host. Host detail charts. Alerts: `gpu_util`, `gpu_temp`, `gpu_mem_pct`.

**Out**: Per-process GPU usage. Intel iGPU (no stable userspace metric). Apple Silicon GPU. Vendor-specific knobs (power limit, fan curves). Docker container GPU passthrough auto-detect — operator mounts `/dev/nvidia*` themselves.

### Process list top-N
**In**: gopsutil-derived list of top N processes by CPU or RAM, configurable per agent. Settings runtime gate (default OFF). Host detail "Top processes" table.

**Out**: Process tree. Per-process network/disk I/O. Container-aware grouping (containers already shown separately). Send / kill from UI — out of scope (anti-feature).

### Maintenance windows
**In**: Schedule a time window with tag selector — alerts engine skips matching rules during the window. Active / upcoming / past listing.

**Out**: Recurring windows in v1 (one-shot only). Auto-acknowledge events during a window. Calendar import (iCal). Per-rule (not per-tag) windows.

## Design

### Shared schema additions

`migrations/0022_gpu_processes_maintenance.sql`:

```sql
-- Maintenance windows
CREATE TABLE maintenance_windows (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    start_at   DATETIME NOT NULL,
    end_at     DATETIME NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    scope_tags TEXT NOT NULL DEFAULT '{}',  -- JSON object {tag: value}
    created_by INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_mw_active ON maintenance_windows(start_at, end_at);
```

GPU + process list are wire-only (no DB rows — live snapshot only). Settings keys added for top-N:

| Key | Default |
|---|---|
| `processes.enabled` | `false` |
| `processes.top_n` | `10` |
| `processes.sort_by` | `cpu` (or `rss`) |

### Agent collectors

`internal/agent/collector/gpu.go`:
- Detect `nvidia-smi` on `$PATH` → query.
- Detect `rocm-smi` on `$PATH` → query.
- Returns `[]api.GPUInfo{Index, Name, UtilPct, MemUsedMB, MemTotalMB, TempC}`.
- Cached executable lookup at startup; missing both = empty slice (no error).

`internal/agent/collector/processes.go`:
- `gopsutil/v4/process.Processes()` → for each PID: cmdline truncated to 200 chars, CPU%, RSS, name, user.
- Sort + truncate to N.
- Returns `[]api.ProcessInfo`.
- Off by default; opt-in via agent env `LUMEN_AGENT_PROCESSES=true` AND server `processes.enabled=true` (defense in depth).

### Schema additions

`internal/shared/api/types.go`:

```go
type GPUInfo struct {
    Index      int    `json:"index"`
    Name       string `json:"name"`
    UtilPct    float64 `json:"util_pct"`
    MemUsedMB  uint64 `json:"mem_used_mb"`
    MemTotalMB uint64 `json:"mem_total_mb"`
    TempC      float64 `json:"temp_c"`
}

type ProcessInfo struct {
    PID    int32   `json:"pid"`
    Name   string  `json:"name"`
    User   string  `json:"user"`
    CPUPct float64 `json:"cpu_pct"`
    RSSMB  uint64  `json:"rss_mb"`
    Cmd    string  `json:"cmd"`
}
```

Extend `HostSnapshot` with `GPUs []GPUInfo` and `Processes []ProcessInfo`.

### Alerts metric registration

`internal/hub/alerts/metrics.go` — new metric types `gpu_util`, `gpu_temp`, `gpu_mem_pct`. Eval handles multi-GPU by alerting on max value across all GPUs of that host (simpler than per-GPU rules; document the choice).

### Maintenance handlers + endpoints

`internal/hub/maintenance/handlers.go`:
- `POST /api/maintenance` — create window.
- `GET /api/maintenance?state=active|upcoming|past` — list filter.
- `PUT /api/maintenance/{id}` — edit window (only if not yet started).
- `DELETE /api/maintenance/{id}` — cancel.

Alerts engine reads cached `[]MaintenanceWindow` and, on transition, checks `matchesScope(rule.Tags, win.ScopeTags) && now ∈ [start,end]` — skip notify + skip event insert (matches the existing host-silence semantics).

### Frontend

- **GPU**: Host detail adds a `GPUSection` rendering one card per GPU with util / mem / temp progress bars + a 1h time-series chart. Rules form adds the three new metric types to the metric selector.
- **Processes**: Host detail adds `ProcessesTable` (sortable). Settings → Runtime adds the on/off + N + sort field.
- **Maintenance**: Alerts adds a new tab `Maintenance` (similar shape to Channels). Create form: from/to (datetime-local inputs, browser-timezone), reason, tag selector. List with state badges (active / upcoming / past) + actions.

i18n: full keys for Maintenance tab (visitor-facing surface in the broader sense — admin only but in EN+VI). GPU labels are mostly numeric so single-string keys suffice. Process table column headers get keys.

## Risks

| Risk | Mitigation |
|---|---|
| `nvidia-smi` shells out per tick → overhead | Cache executable path; tick interval is already 5-30 s; `nvidia-smi` runs in <100 ms. Document. |
| `rocm-smi` JSON output varies across driver versions | Pin to fields present in ROCm ≥ 5.0; reject older with a warn log. |
| Process cmdline leaks env-var-style secrets | Default-off; opt-in requires both server and agent flag; document explicitly in `docs/configure/processes.md`. |
| Multi-GPU alerts confuse operators | Document "alerts fire on the worst GPU across the host". Provide a per-GPU tag override knob in v2 if anyone asks. |
| Maintenance window edit while active = surprise un-mute | Disallow start_at edit on active windows; allow end_at extension only. |
| Timezone confusion | Storage UTC; UI converts via `Date`; form uses `datetime-local` which is browser-tz. Tested manually across TZ. |

## Testing

- `gpu_test.go` — parse fixture stdout from `nvidia-smi --query-gpu=... --format=csv,noheader`; parse fixture `rocm-smi --json`; missing executable = empty slice.
- `processes_test.go` — fixture process list sort by CPU vs RSS; top-N truncation.
- `maintenance_test.go` — scope match (subset of tags), window active check at edge of start_at + end_at, edit guards.

## Docs deliverables

- `docs/configure/gpu.md` — NVIDIA driver install pointers, ROCm install pointers, Docker container caveats.
- `docs/configure/processes.md` — what's collected, what's NOT, security trade-off.
- `docs/configure/maintenance-windows.md` — when to use, tag scope semantics, timezone behaviour.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. Should maintenance windows also suppress notification delivery for events that ALREADY fired before the window started? Proposed: no — once a notification is queued the queue ships it; window prevents *new* firings.
2. Should the process list ship to the hub when `Cmd` matches an admin-defined regex blocklist (e.g. cmdlines containing `password=`)? Proposed: yes, redact `Cmd` to `<redacted>` if it matches `processes.redact_regex` setting.
