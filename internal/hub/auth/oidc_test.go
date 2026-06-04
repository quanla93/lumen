package auth

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
)

// Save → Load round-trips every field, including the encrypted client
// secret. A subsequent Save with an empty client_secret must KEEP the
// existing one (UX requirement: operator doesn't have to retype on edit).
func TestOIDCConfigRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer db.Close()
	hubSecret := []byte("test-hub-secret-32-bytes-aaaaaaa")

	in := OIDCConfig{
		Enabled:       true,
		Issuer:        "https://idp.example.com/realms/lumen/",
		ClientID:      "client-abc",
		ClientSecret:  "client-secret-xyz",
		Scopes:        "openid email groups",
		ExpectedEmail: "Admin@Example.COM",
	}
	if err := SaveOIDCConfig(ctx, db, hubSecret, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadOIDCConfig(ctx, db, hubSecret, true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := OIDCConfig{
		Enabled:       true,
		Issuer:        "https://idp.example.com/realms/lumen", // trailing slash trimmed
		ClientID:      "client-abc",
		ClientSecret:  "client-secret-xyz",
		Scopes:        "openid email groups",
		ExpectedEmail: "admin@example.com", // lower-cased
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("roundtrip\n got  %+v\n want %+v", out, want)
	}

	// Re-save with empty client_secret keeps the existing one.
	in2 := in
	in2.ClientSecret = ""
	in2.ExpectedEmail = "different@example.com"
	if err := SaveOIDCConfig(ctx, db, hubSecret, in2); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	out2, err := LoadOIDCConfig(ctx, db, hubSecret, true)
	if err != nil {
		t.Fatalf("re-load: %v", err)
	}
	if out2.ClientSecret != "client-secret-xyz" {
		t.Errorf("client_secret lost on empty re-save: got %q", out2.ClientSecret)
	}
	if out2.ExpectedEmail != "different@example.com" {
		t.Errorf("expected_email not updated: got %q", out2.ExpectedEmail)
	}
}

func TestOIDCDefaultScopes(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Settings table is empty → LoadOIDCConfig should fall back to the default scopes.
	cfg, err := LoadOIDCConfig(ctx, db, []byte("anything"), false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled {
		t.Fatal("enabled should default to false")
	}
	if cfg.Scopes != "openid email profile" {
		t.Errorf("default scopes = %q, want %q", cfg.Scopes, "openid email profile")
	}
	// Sanity: verify settings.Get behaviour for our keys.
	if v, _ := settings.Get(ctx, db, OIDCKeyEnabled); v != "" {
		t.Errorf("OIDCKeyEnabled before save = %q, want empty", v)
	}
}

// splitScopes accepts comma-, space-, or mixed-separated input and
// drops the empties so operators can paste from anywhere.
func TestSplitScopes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"openid email profile", []string{"openid", "email", "profile"}},
		{"openid,email,profile", []string{"openid", "email", "profile"}},
		{"openid, email , profile", []string{"openid", "email", "profile"}},
		{"", []string{"openid", "email", "profile"}},  // empty -> defaults
		{"  ", []string{"openid", "email", "profile"}}, // whitespace-only -> defaults
		{"openid", []string{"openid"}},
	}
	for _, tc := range cases {
		got := splitScopes(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitScopes(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
