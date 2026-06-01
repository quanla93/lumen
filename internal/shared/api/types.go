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
	CpuSeries  []float64       `json:"cpu_series,omitempty"`
}
