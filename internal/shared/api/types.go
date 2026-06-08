// Package api defines the wire types shared between the Lumen hub and agent.
//
// These types form the v0 ingest envelope. Phase 2 will version them under
// /api/v1/ once the field set stabilizes. Today the field set evolves with
// each phase (pre-1.0 allows breaks per ACTION_PLAN).
package api

import "time"

// AgentPolicyResponse is returned by the hub control-plane endpoint agents
// poll to learn runtime operator policy without redeploying.
type AgentPolicyResponse struct {
	CollectionInterval string `json:"collection_interval"`
	// ProcessesEnabled gates the per-host process list collector.
	// Defense in depth: the agent also requires LUMEN_AGENT_PROCESSES=true
	// at boot. Both flags AND together — the collector is only active
	// when the operator has opted in at both ends.
	ProcessesEnabled bool `json:"processes_enabled"`
	// ProcessesTopN is the cap on rows the agent ships. Default 10,
	// max 50 (RFC 0003).
	ProcessesTopN int `json:"processes_top_n"`
	// ProcessesSortBy is "cpu" or "rss". Default "cpu".
	ProcessesSortBy string `json:"processes_sort_by"`
	// ProcessesRedactRegex overrides the defensive default that
	// catches password=... style secrets in cmdline. Empty = use
	// the built-in default (RFC 0003 Q2).
	ProcessesRedactRegex string `json:"processes_redact_regex,omitempty"`
}

// ContainerInfo is one row of the running-container snapshot the agent
// ships when the local Docker daemon is reachable. NOT persisted to
// SQLite (variable cardinality per host, ephemeral by nature) — flows
// only through the in-memory store + WS broadcast so the host detail
// page can render a live list.
//
// Mem fields are bytes (raw, not %); MemPct is precomputed
// (used / limit * 100) so the UI doesn't have to guard against
// limit==0 in every cell.
type ContainerInfo struct {
	ID            string  `json:"id"`   // short, 12-char
	Name          string  `json:"name"` // leading "/" stripped
	Image         string  `json:"image"`
	State         string  `json:"state"` // running, paused, exited, restarting, ...
	CpuPct        float64 `json:"cpu_pct"`
	MemUsedBytes  uint64  `json:"mem_used_bytes"`
	MemLimitBytes uint64  `json:"mem_limit_bytes"`
	MemPct        float64 `json:"mem_pct"`
}

// SystemMetadata is the latest host/agent identity context. The hub stores
// only the newest value per host; it is not historical time-series data.
type SystemMetadata struct {
	OS            string `json:"os,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	PrimaryIP     string `json:"primary_ip,omitempty"`
	Kernel        string `json:"kernel,omitempty"`
	Arch          string `json:"arch,omitempty"`
	CPUModel      string `json:"cpu_model,omitempty"`
	UptimeSeconds uint64 `json:"uptime_seconds,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	// VirtType is gopsutil's VirtualizationSystem ("kvm", "lxc", "docker",
	// "vmware", "wsl", …) when the agent runs in a guest; empty on bare
	// metal. Empty also means "older agent that didn't report it" —
	// callers treating "unknown" as bare-metal is the safe default
	// (show per-core; operator can verify by clicking through).
	VirtType string `json:"virt_type,omitempty"`
}

// IngestRequest is the body of POST /api/ingest sent by the agent every tick.
// Metric fields default to zero when the agent couldn't collect them — the
// hub keeps zeros rather than dropping the whole sample so partial data
// (e.g. no load-avg on Windows) still moves the timestamp forward.
//
// Rate fields (Net*Bps, Disk*Bps) are bytes-per-second computed by the
// agent from cumulative gopsutil counters across two ticks. On the very
// first tick after agent start there's no prior counter to diff against,
// so the agent sends 0 for those rates.
//
// CpuPerCore is included in the envelope but NOT persisted to SQLite —
// per-core data only flows through the in-memory hot path (WS stream)
// to keep storage simple and HDD-friendly.
type IngestRequest struct {
	Host       string          `json:"host"`
	Ts         time.Time       `json:"ts"`
	CpuPct     float64         `json:"cpu_pct"`
	CpuPerCore []float64       `json:"cpu_per_core,omitempty"`
	RamPct     float64         `json:"ram_pct"`
	SwapPct    float64         `json:"swap_pct"`
	DiskPct    float64         `json:"disk_pct"`
	Load1      float64         `json:"load1"`
	Load5      float64         `json:"load5"`
	Load15     float64         `json:"load15"`
	NetRxBps   float64         `json:"net_rx_bps"`
	NetTxBps   float64         `json:"net_tx_bps"`
	DiskRBps   float64         `json:"disk_r_bps"`
	DiskWBps   float64         `json:"disk_w_bps"`
	TempC      float64         `json:"temp_c"`
	Containers []ContainerInfo `json:"containers,omitempty"`
	System     SystemMetadata  `json:"system,omitempty"`
	GPUs       []GPUInfo       `json:"gpus,omitempty"`
	Processes  []ProcessInfo   `json:"processes,omitempty"`
}

// StreamControl is a client → server message on /api/stream. Today only
// one verb exists; the wrapper struct exists so we can grow the
// protocol (acks, pings, server-side filters) without breaking
// existing clients.
//
//	{"type":"subscribe","hosts":["webA","webB"]}  // filter to these
//	{"type":"subscribe","hosts":["*"]}            // unfiltered (default)
//
// A connection that never sends a control frame stays in the default
// "all hosts" mode — older web builds keep working with no changes.
type StreamControl struct {
	Type  string   `json:"type"`
	Hosts []string `json:"hosts,omitempty"`
}

// HostSnapshot is the latest known state of a single host as held by the hub.
// CpuSeries is the recent CPU history (oldest first) the hub kept in its
// per-host ring buffer; clients use it to draw sparklines without a
// cold-start gap on connect.
type HostSnapshot struct {
	Host string `json:"host"`
	// Ts is the agent's collection time — drifts arbitrarily into the
	// past while a backlog is draining after hub downtime.
	Ts time.Time `json:"ts"`
	// ReceivedAt is server-stamped at ingest. Use this — not Ts — to
	// decide whether the host is online: drained backlog frames carry
	// stale Ts but fresh ReceivedAt, so liveness keyed on Ts would
	// show "offline" while data is actively flowing in.
	ReceivedAt time.Time       `json:"received_at"`
	CpuPct     float64         `json:"cpu_pct"`
	CpuPerCore []float64       `json:"cpu_per_core,omitempty"`
	RamPct     float64         `json:"ram_pct"`
	SwapPct    float64         `json:"swap_pct"`
	DiskPct    float64         `json:"disk_pct"`
	Load1      float64         `json:"load1"`
	Load5      float64         `json:"load5"`
	Load15     float64         `json:"load15"`
	NetRxBps   float64         `json:"net_rx_bps"`
	NetTxBps   float64         `json:"net_tx_bps"`
	DiskRBps   float64         `json:"disk_r_bps"`
	DiskWBps   float64         `json:"disk_w_bps"`
	TempC      float64         `json:"temp_c"`
	Containers []ContainerInfo `json:"containers,omitempty"`
	System     SystemMetadata  `json:"system,omitempty"`
	CpuSeries  []float64       `json:"cpu_series,omitempty"`
	// GPUs is live-only (not persisted) — the host detail page reads
	// it for the per-GPU section + chart, and the alerts engine
	// extracts worst-of values for the gpu_util / gpu_mem / gpu_temp
	// metric types. Same lifecycle as CpuPerCore: variable-cardinality
	// per host, no historical bucketing.
	GPUs []GPUInfo `json:"gpus,omitempty"`
	// Processes is live-only + opt-in (default OFF). See RFC 0003
	// for the cmdline-leaks-secrets trade-off; the agent gates on
	// LUMEN_AGENT_PROCESSES=true AND the server gate at
	// processes.enabled=true (defense in depth).
	Processes []ProcessInfo `json:"processes,omitempty"`
}

// GPUInfo is one physical GPU on a host. Multi-GPU hosts have a
// slice of these; the alerts engine fires on the worst value
// across the slice (documented in docs/configure/gpu.md).
type GPUInfo struct {
	Index      int     `json:"index"`
	Name       string  `json:"name"`
	UtilPct    float64 `json:"util_pct"`
	MemUsedMB  uint64  `json:"mem_used_mb"`
	MemTotalMB uint64  `json:"mem_total_mb"`
	TempC      float64 `json:"temp_c"`
}

// ProcessInfo is one row in the host detail "Top processes" table.
// Cmd is truncated to 200 chars at the agent and may be redacted
// server-side if it matches processes.redact_regex (RFC 0003 Q2
// proposed: defensive default that catches password=... style
// secrets even when the operator opts in).
type ProcessInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	User   string  `json:"user"`
	CPUPct float64 `json:"cpu_pct"`
	RSSMB  uint64  `json:"rss_mb"`
	Cmd    string  `json:"cmd"`
}
