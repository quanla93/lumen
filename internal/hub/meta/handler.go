// Package meta exposes hub build/version metadata for the web UI.
package meta

import (
	"encoding/json"
	"net/http"
)

// Response is the JSON body of GET /api/version.
//
// HubVersion is the running hub build. LatestAgentVersion is the newest
// agent version this hub can vouch for — because the hub and agent ship
// from the same release train, that is simply the hub's own version. The
// UI compares each host's reported agent_version against this to flag
// out-of-date agents.
type Response struct {
	HubVersion         string `json:"hub_version"`
	LatestAgentVersion string `json:"latest_agent_version"`
}

// Handler serves GET /api/version.
type Handler struct {
	HubVersion string
}

// New returns a Handler. An empty hubVersion falls back to "dev" so source
// builds (no -ldflags) report something coherent instead of an empty string.
func New(hubVersion string) *Handler {
	if hubVersion == "" {
		hubVersion = "dev"
	}
	return &Handler{HubVersion: hubVersion}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{
		HubVersion:         h.HubVersion,
		LatestAgentVersion: h.HubVersion,
	})
}
