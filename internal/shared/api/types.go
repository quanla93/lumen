// Package api defines the wire types shared between the Lumen hub and agent.
//
// These types form the v0 ingest envelope. Phase 2 will version them under
// /api/v1/ once the field set stabilizes. Today the field set evolves with
// each phase (pre-1.0 allows breaks per ACTION_PLAN).
package api

import "time"

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
	Host       string    `json:"host"`
	Ts         time.Time `json:"ts"`
	CpuPct     float64   `json:"cpu_pct"`
	CpuPerCore []float64 `json:"cpu_per_core,omitempty"`
	RamPct     float64   `json:"ram_pct"`
	SwapPct    float64   `json:"swap_pct"`
	DiskPct    float64   `json:"disk_pct"`
	Load1      float64   `json:"load1"`
	Load5      float64   `json:"load5"`
	Load15     float64   `json:"load15"`
	NetRxBps   float64   `json:"net_rx_bps"`
	NetTxBps   float64   `json:"net_tx_bps"`
	DiskRBps   float64   `json:"disk_r_bps"`
	DiskWBps   float64   `json:"disk_w_bps"`
	TempC      float64   `json:"temp_c"`
}

// HostSnapshot is the latest known state of a single host as held by the hub.
// CpuSeries is the recent CPU history (oldest first) the hub kept in its
// per-host ring buffer; clients use it to draw sparklines without a
// cold-start gap on connect.
type HostSnapshot struct {
	Host       string    `json:"host"`
	Ts         time.Time `json:"ts"`
	CpuPct     float64   `json:"cpu_pct"`
	CpuPerCore []float64 `json:"cpu_per_core,omitempty"`
	RamPct     float64   `json:"ram_pct"`
	SwapPct    float64   `json:"swap_pct"`
	DiskPct    float64   `json:"disk_pct"`
	Load1      float64   `json:"load1"`
	Load5      float64   `json:"load5"`
	Load15     float64   `json:"load15"`
	NetRxBps   float64   `json:"net_rx_bps"`
	NetTxBps   float64   `json:"net_tx_bps"`
	DiskRBps   float64   `json:"disk_r_bps"`
	DiskWBps   float64   `json:"disk_w_bps"`
	TempC      float64   `json:"temp_c"`
	CpuSeries  []float64 `json:"cpu_series,omitempty"`
}
