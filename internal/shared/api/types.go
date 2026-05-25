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
type IngestRequest struct {
	Host    string    `json:"host"`
	Ts      time.Time `json:"ts"`
	CpuPct  float64   `json:"cpu_pct"`
	RamPct  float64   `json:"ram_pct"`
	SwapPct float64   `json:"swap_pct"`
	DiskPct float64   `json:"disk_pct"`
	Load1   float64   `json:"load1"`
	Load5   float64   `json:"load5"`
	Load15  float64   `json:"load15"`
}

// HostSnapshot is the latest known state of a single host as held by the hub.
// CpuSeries is the recent CPU history (oldest first) the hub kept in its
// per-host ring buffer; clients use it to draw sparklines without a
// cold-start gap on connect.
type HostSnapshot struct {
	Host      string    `json:"host"`
	Ts        time.Time `json:"ts"`
	CpuPct    float64   `json:"cpu_pct"`
	RamPct    float64   `json:"ram_pct"`
	SwapPct   float64   `json:"swap_pct"`
	DiskPct   float64   `json:"disk_pct"`
	Load1     float64   `json:"load1"`
	Load5     float64   `json:"load5"`
	Load15    float64   `json:"load15"`
	CpuSeries []float64 `json:"cpu_series,omitempty"`
}
