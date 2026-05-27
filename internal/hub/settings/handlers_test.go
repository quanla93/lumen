package settings

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quanla93/lumen/internal/hub/storage"
)

func TestSettingsPutUpdatesAgentInterval(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()
	if err := EnsureDefaults(context.Background(), db, map[string]string{
		KeyRetentionWindow:   "24h",
		KeyRetentionInterval: "1h",
		KeyAgentInterval:     "5s",
	}); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"agent_interval":"30s"}`))
	rr := httptest.NewRecorder()

	NewHandlers(db, nil).Put(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	got, err := Get(context.Background(), db, KeyAgentInterval)
	if err != nil {
		t.Fatalf("get interval: %v", err)
	}
	if got != "30s" {
		t.Fatalf("agent interval = %q, want 30s", got)
	}
	if !strings.Contains(rr.Body.String(), `"agent_interval":"30s"`) {
		t.Fatalf("response body missing updated interval: %s", rr.Body.String())
	}
}

func TestSettingsPutRejectsTooFastAgentInterval(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"agent_interval":"1s"}`))
	rr := httptest.NewRecorder()

	NewHandlers(db, nil).Put(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "agent_interval") {
		t.Fatalf("response body missing field name: %s", rr.Body.String())
	}
}

func TestSettingsPutUpdatesDownsamplePolicy(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()
	if err := EnsureDefaults(context.Background(), db, map[string]string{
		KeyRetentionWindow:         "24h",
		KeyRetentionInterval:       "1h",
		KeyAgentInterval:           "5s",
		KeyDownsampleBucketSize:    "5m",
		KeyDownsampleHotWindow:     "24h",
		KeyDownsampleArchiveWindow: "8760h",
	}); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"downsample_bucket_size":"10m","downsample_hot_window":"48h","downsample_archive_window":"720h"}`))
	rr := httptest.NewRecorder()

	NewHandlers(db, nil).Put(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	assertSetting(t, db, KeyDownsampleBucketSize, "10m")
	assertSetting(t, db, KeyDownsampleHotWindow, "48h")
	assertSetting(t, db, KeyDownsampleArchiveWindow, "720h")
	for _, want := range []string{
		`"downsample_bucket_size":"10m"`,
		`"downsample_hot_window":"48h"`,
		`"downsample_archive_window":"720h"`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("response body missing %s: %s", want, rr.Body.String())
		}
	}
}

func TestSettingsPutRejectsInvalidDownsamplePolicy(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"bucket too small", `{"downsample_bucket_size":"30s"}`, "downsample_bucket_size"},
		{"hot window too small", `{"downsample_hot_window":"30m"}`, "downsample_hot_window"},
		{"archive window too small", `{"downsample_archive_window":"12h"}`, "downsample_archive_window"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
			if err != nil {
				t.Fatalf("open storage: %v", err)
			}
			defer db.Close()

			req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()

			NewHandlers(db, nil).Put(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("response body missing field name: %s", rr.Body.String())
			}
		})
	}
}

func assertSetting(t *testing.T, db *sql.DB, key, want string) {
	t.Helper()
	got, err := Get(context.Background(), db, key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
