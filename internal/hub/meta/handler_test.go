package meta

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerReturnsHubAndAgentVersion(t *testing.T) {
	h := New("v0.2.0")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var got Response
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Hub and agent ship together, so latest agent version mirrors the hub.
	if got.HubVersion != "v0.2.0" || got.LatestAgentVersion != "v0.2.0" {
		t.Fatalf("got %+v, want hub and agent both v0.2.0", got)
	}
}

func TestNewDefaultsEmptyToDev(t *testing.T) {
	if h := New(""); h.HubVersion != "dev" {
		t.Fatalf("HubVersion = %q, want dev", h.HubVersion)
	}
}
