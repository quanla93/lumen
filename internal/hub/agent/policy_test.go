package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
	"github.com/quanla93/lumen/internal/shared/api"
)

func TestPolicyHandlerReturnsConfiguredCollectionInterval(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()

	_, token, err := hosts.Create(context.Background(), db, "pve-01")
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	if err := settings.Set(context.Background(), db, settings.KeyAgentInterval, "15s"); err != nil {
		t.Fatalf("set interval: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agent/policy", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	NewPolicyHandler(db, nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got api.AgentPolicyResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CollectionInterval != "15s" {
		t.Fatalf("collection_interval = %q, want 15s", got.CollectionInterval)
	}
}

func TestPolicyHandlerRejectsInvalidToken(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/agent/policy", nil)
	req.Header.Set("Authorization", "Bearer lum_invalid")
	rr := httptest.NewRecorder()

	NewPolicyHandler(db, nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
