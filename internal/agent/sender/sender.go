// Package sender ships ingest envelopes from the agent to the hub.
//
// Phase 1: synchronous HTTP POST. Phase 2 adds the offline-buffer / retry
// path via BoltDB so the agent can survive hub downtime.
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/quanla93/lumen/internal/shared/api"
)

type Sender struct {
	HubURL string
	Token  string
	HTTP   *http.Client
}

func New(hubURL, token string) *Sender {
	return &Sender{
		HubURL: strings.TrimRight(hubURL, "/"),
		Token:  token,
		HTTP:   &http.Client{Timeout: 5 * time.Second},
	}
}

// FetchPolicy gets runtime operator policy from the hub. It is best-effort;
// callers should keep their current local config if this fails.
func (s *Sender) FetchPolicy(ctx context.Context) (api.AgentPolicyResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.HubURL+"/api/agent/policy", nil)
	if err != nil {
		return api.AgentPolicyResponse{}, fmt.Errorf("build policy request: %w", err)
	}
	if s.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.Token)
	}
	resp, err := s.HTTP.Do(httpReq)
	if err != nil {
		return api.AgentPolicyResponse{}, fmt.Errorf("get policy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return api.AgentPolicyResponse{}, fmt.Errorf("hub returned %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	var out api.AgentPolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return api.AgentPolicyResponse{}, fmt.Errorf("decode policy: %w", err)
	}
	return out, nil
}

// Send POSTs one ingest envelope to the hub.
func (s *Sender) Send(ctx context.Context, req api.IngestRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.HubURL+"/api/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.Token != "" {
		// Auth not enforced until Phase 2 — we send the header so the wire
		// format is stable when the hub starts validating.
		httpReq.Header.Set("Authorization", "Bearer "+s.Token)
	}

	resp, err := s.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("hub returned %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return nil
}
