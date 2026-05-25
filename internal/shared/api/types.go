// Package api defines the wire types shared between the Lumen hub and agent.
//
// These types form the v0 ingest envelope. Phase 2 will version them under
// /api/v1/ and add more metric fields; for the Phase 1 spike only CpuPct
// is populated.
package api

import "time"

// IngestRequest is the body of POST /api/ingest sent by the agent every tick.
type IngestRequest struct {
	Host   string    `json:"host"`
	Ts     time.Time `json:"ts"`
	CpuPct float64   `json:"cpu_pct"`
}

// HostSnapshot is the latest known state of a single host as held by the hub.
type HostSnapshot struct {
	Host   string    `json:"host"`
	Ts     time.Time `json:"ts"`
	CpuPct float64   `json:"cpu_pct"`
}
