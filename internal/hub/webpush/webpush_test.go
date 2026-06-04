package webpush

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/quanla93/lumen/internal/hub/storage"
)

// EnsureKeys must be idempotent: the second call returns the same pair
// so a hub restart (or two browsers subscribing in rapid succession)
// doesn't invalidate existing client subscriptions.
func TestEnsureKeysIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	secret := []byte("hub-secret-32-bytes-aaaaaaaaaaaa")

	k1, err := EnsureKeys(ctx, db, secret)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if k1.PublicKey == "" || k1.PrivateKey == "" {
		t.Fatal("expected keys populated")
	}
	k2, err := EnsureKeys(ctx, db, secret)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if k1.PublicKey != k2.PublicKey || k1.PrivateKey != k2.PrivateKey {
		t.Fatal("EnsureKeys re-rolled the pair — would invalidate every browser")
	}
}

// Decrypting the stored VAPID private key with the wrong hub secret
// must fail loudly, not silently return garbage that the push library
// would later reject with an opaque "invalid signature".
func TestLoadKeysWrongSecret(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := EnsureKeys(ctx, db, []byte("real-secret-32-bytes-aaaaaaaaaaa")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadKeys(ctx, db, []byte("WRONG-secret-32-bytes-bbbbbbbbbb")); err == nil {
		t.Fatal("LoadKeys with the wrong hub secret should error, not return zero")
	}
}

// AddSubscription must dedupe on (channel_id, endpoint) so a browser
// resubscribing (after a page reload or service-worker update) reuses
// the existing row instead of growing the table without bound.
func TestAddSubscriptionDedup(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	channelID := seedChannel(t, db)

	in := Subscription{
		ChannelID: channelID,
		Endpoint:  "https://example.invalid/push/abc",
		P256dh:    "p256dh-1",
		Auth:      "auth-1",
		Label:     "Chrome",
	}
	first, err := AddSubscription(ctx, db, in)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	in.Auth = "auth-2"
	in.Label = "Chrome updated"
	second, err := AddSubscription(ctx, db, in)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("upsert created a new row (id %d vs %d) — should have updated", first.ID, second.ID)
	}
	subs, err := ListSubscriptions(ctx, db, channelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after upsert, got %d", len(subs))
	}
	if subs[0].Auth != "auth-2" || subs[0].Label != "Chrome updated" {
		t.Fatalf("upsert did not overwrite fields: %+v", subs[0])
	}
}

// seedChannel inserts a minimal notification_channels row via raw SQL
// so the FK constraint on web_push_subscriptions is satisfied. We
// don't reach into the alerts package — the webpush package's tests
// shouldn't depend on alerts' channel-validation machinery.
func seedChannel(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO notification_channels (name, type, config, owner_type, enabled, min_severity)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"test-push", "web_push", "{}", "admin", 1, "info",
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed channel last id: %v", err)
	}
	return id
}
