package publicstatus

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/hub/storage"
)

// disabled: handler returns 200 with enabled=false even when no rows
// exist. This is contractual: the frontend renders a deterministic
// "not published" notice off this shape and shouldn't have to handle
// 404 separately.
func TestGetDisabled(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New()
	h := NewHandlers(db, st, slog.New(slog.DiscardHandler))

	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/public/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp publicStatusResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Enabled {
		t.Fatal("Enabled should be false on a fresh DB")
	}
	if resp.Title != "Status" {
		t.Errorf("default Title = %q, want %q", resp.Title, "Status")
	}
	if len(resp.Hosts) != 0 {
		t.Errorf("Hosts on disabled = %d, want 0", len(resp.Hosts))
	}
}

// enabled with no opted-in hosts: returns enabled=true and an empty
// (non-nil) Hosts slice so the frontend renders "No hosts are public
// yet" deterministically.
func TestGetEnabledEmpty(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New()
	h := NewHandlers(db, st, slog.New(slog.DiscardHandler))

	if err := SaveConfig(ctx, db, Config{Enabled: true, Title: "Fleet", Description: "lab"}); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/public/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp publicStatusResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Enabled || resp.Title != "Fleet" || resp.Description != "lab" {
		t.Fatalf("config not applied: %+v", resp)
	}
	if resp.Hosts == nil {
		t.Error("Hosts must be non-nil even when empty")
	}
}

// per-host opt-in: only hosts with public_visible=1 appear, regardless
// of how many exist in total. The handler joins live snapshot CPU/RAM/
// disk in via the in-memory store.
func TestGetOnlyVisibleHosts(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New()

	if err := SaveConfig(ctx, db, Config{Enabled: true, Title: "S"}); err != nil {
		t.Fatal(err)
	}
	publicHost, _, err := hosts.Create(ctx, db, "public-host")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hosts.Create(ctx, db, "private-host"); err != nil {
		t.Fatal(err)
	}
	if err := hosts.SetPublicVisible(ctx, db, publicHost.ID, true); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(db, st, slog.New(slog.DiscardHandler))
	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/public/status", nil))

	var resp publicStatusResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hosts) != 1 {
		t.Fatalf("Hosts = %d, want 1; got %+v", len(resp.Hosts), resp.Hosts)
	}
	if resp.Hosts[0].Name != "public-host" {
		t.Errorf("got %q, want public-host", resp.Hosts[0].Name)
	}
	// No snapshot in store → state is "down" because we have a last_seen
	// already (Create sets it via the LastSeen path? no — defaults nil).
	// Without LastSeen + without snapshot, state should remain "unknown".
	if resp.Hosts[0].State != "unknown" {
		t.Errorf("State = %q, want unknown for a never-seen host with no snapshot", resp.Hosts[0].State)
	}
}
