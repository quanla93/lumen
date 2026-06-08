// Package webpush owns the VAPID key pair, browser subscription store,
// and the dispatcher's path to the Push Service.
//
// VAPID = "Voluntary Application Server Identification for Web Push" —
// the W3C Push API requires every push to be signed with a JWT proving
// the sender owns the public key the browser already trusts. The hub
// generates ONE key pair on first need and stores it in the settings
// table; rotating it invalidates every existing browser subscription,
// so we keep it stable for the life of the deployment.
//
// The private key is AES-GCM-encrypted at rest using the same KEK
// derivation as the OIDC client secret (see internal/hub/auth/crypto.go).
// LUMEN_HUB_SECRET must be stable across restarts for both that and this.
package webpush

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	gowebpush "github.com/SherClockHolmes/webpush-go"

	"github.com/quanla93/lumen/internal/hub/settings"
)

// Settings keys for the hub's singleton VAPID pair + the "subject" the
// JWT carries (RFC 8292 §2.1: a URL or mailto: that the push service
// can reach if delivery goes wrong).
const (
	SettingsKeyVAPIDPublic     = "webpush.vapid_public_key"
	SettingsKeyVAPIDPrivateEnc = "webpush.vapid_private_key_enc"
	SettingsKeyVAPIDSubject    = "webpush.vapid_subject"
)

// DefaultSubject is used until the operator sets one explicitly. RFC
// 8292 requires either a https:// URL or a mailto: scheme.
const DefaultSubject = "mailto:admin@example.invalid"

// Keys is the loaded, decoded VAPID material. PublicKey is what the
// frontend's PushManager.subscribe() needs; PrivateKey + Subject are
// what we hand to the gowebpush sender on each push.
type Keys struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// EnsureKeys generates a fresh VAPID pair the first time it's called
// and returns the existing pair on subsequent calls. Idempotent. The
// frontend can call GetPublicKey directly without invoking generation;
// generation happens at channel-create time so a deployment that never
// enables web push never pays the cost.
func EnsureKeys(ctx context.Context, db *sql.DB, hubSecret []byte) (Keys, error) {
	if k, ok, err := LoadKeys(ctx, db, hubSecret); err != nil {
		return Keys{}, err
	} else if ok {
		return k, nil
	}
	priv, pub, err := gowebpush.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, fmt.Errorf("generate VAPID: %w", err)
	}
	encPriv, err := encryptVAPIDPrivate(priv, hubSecret)
	if err != nil {
		return Keys{}, err
	}
	if err := settings.Set(ctx, db, SettingsKeyVAPIDPublic, pub); err != nil {
		return Keys{}, err
	}
	if err := settings.Set(ctx, db, SettingsKeyVAPIDPrivateEnc, encPriv); err != nil {
		return Keys{}, err
	}
	subject, _ := settings.Get(ctx, db, SettingsKeyVAPIDSubject)
	if subject == "" {
		subject = DefaultSubject
		_ = settings.Set(ctx, db, SettingsKeyVAPIDSubject, subject)
	}
	return Keys{PublicKey: pub, PrivateKey: priv, Subject: subject}, nil
}

// LoadKeys returns the existing pair, or (zero, false, nil) if none has
// been generated yet. Used by the public-key endpoint so a fresh hub
// can answer "we don't have one yet" without paying generation cost.
func LoadKeys(ctx context.Context, db *sql.DB, hubSecret []byte) (Keys, bool, error) {
	pub, _ := settings.Get(ctx, db, SettingsKeyVAPIDPublic)
	encPriv, _ := settings.Get(ctx, db, SettingsKeyVAPIDPrivateEnc)
	if pub == "" || encPriv == "" {
		return Keys{}, false, nil
	}
	priv, err := decryptVAPIDPrivate(encPriv, hubSecret)
	if err != nil {
		return Keys{}, false, err
	}
	subject, _ := settings.Get(ctx, db, SettingsKeyVAPIDSubject)
	if subject == "" {
		subject = DefaultSubject
	}
	return Keys{PublicKey: pub, PrivateKey: priv, Subject: subject}, true, nil
}

// SetSubject updates the VAPID subject (must be https:// or mailto:).
func SetSubject(ctx context.Context, db *sql.DB, subject string) error {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "https://") && !strings.HasPrefix(subject, "mailto:") {
		return errors.New("VAPID subject must start with https:// or mailto")
	}
	return settings.Set(ctx, db, SettingsKeyVAPIDSubject, subject)
}

// Subscription is one browser registration backing a web_push channel.
type Subscription struct {
	ID        int64
	ChannelID int64
	Endpoint  string
	P256dh    string
	Auth      string
	Label     string
	CreatedAt time.Time
}

// ListSubscriptions returns all subscriptions for a channel, oldest first.
func ListSubscriptions(ctx context.Context, db *sql.DB, channelID int64) ([]Subscription, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, channel_id, endpoint, p256dh, auth, label, created_at
		 FROM web_push_subscriptions WHERE channel_id = ? ORDER BY created_at ASC`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.ChannelID, &s.Endpoint, &s.P256dh, &s.Auth, &s.Label, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddSubscription stores a new subscription. A duplicate (channel_id +
// endpoint) is a no-op so the frontend can resubscribe idempotently.
func AddSubscription(ctx context.Context, db *sql.DB, s Subscription) (Subscription, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO web_push_subscriptions (channel_id, endpoint, p256dh, auth, label)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(channel_id, endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth, label = excluded.label`,
		s.ChannelID, s.Endpoint, s.P256dh, s.Auth, s.Label,
	)
	if err != nil {
		return Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		// Upsert hit the conflict path — look up the existing row.
		err := db.QueryRowContext(ctx,
			`SELECT id, channel_id, endpoint, p256dh, auth, label, created_at
			 FROM web_push_subscriptions WHERE channel_id = ? AND endpoint = ?`,
			s.ChannelID, s.Endpoint,
		).Scan(&s.ID, &s.ChannelID, &s.Endpoint, &s.P256dh, &s.Auth, &s.Label, &s.CreatedAt)
		return s, err
	}
	s.ID = id
	s.CreatedAt = time.Now().UTC()
	return s, nil
}

// DeleteSubscription drops a single browser registration. The
// dispatcher calls this when the push service returns 404/410 (the
// browser unsubscribed on its side).
func DeleteSubscription(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM web_push_subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SendOne dispatches one push to one subscription. Returns a wrapped
// error containing the push service's HTTP status when the response
// is non-2xx so the dispatcher can decide whether to drop the
// subscription (404/410) or retry (transient).
func SendOne(ctx context.Context, keys Keys, sub Subscription, payload []byte) error {
	gws := &gowebpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     gowebpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}
	resp, err := gowebpush.SendNotificationWithContext(ctx, payload, gws, &gowebpush.Options{
		Subscriber:      keys.Subject,
		VAPIDPublicKey:  keys.PublicKey,
		VAPIDPrivateKey: keys.PrivateKey,
		TTL:             30, // seconds. Drop quickly if the browser is offline; alerts care about now.
		Topic:           "lumen-alert",
		Urgency:         gowebpush.UrgencyHigh,
	})
	if err != nil {
		return fmt.Errorf("web push send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &PushError{Status: resp.StatusCode}
}

// PushError carries the push service's HTTP status so dispatchers can
// branch on Gone (drop) vs transient (retry).
type PushError struct {
	Status int
}

func (e *PushError) Error() string { return fmt.Sprintf("push service returned %d", e.Status) }

// IsGone returns true when the push service says the subscription is
// permanently invalid (404 or 410). The dispatcher uses this to delete
// the row so we stop retrying a dead endpoint.
func IsGone(err error) bool {
	var pe *PushError
	if errors.As(err, &pe) {
		return pe.Status == 404 || pe.Status == 410
	}
	return false
}

// ── private key encryption ──────────────────────────────────────────
//
// Same scheme as auth.EncryptSecret but a different KEK label so the
// two encrypted blobs are mutually unintelligible. Kept local to avoid
// a webpush → auth dependency (which would loop through settings).

func encryptVAPIDPrivate(plaintext string, hubSecret []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := newWebPushAEAD(hubSecret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

func decryptVAPIDPrivate(ciphertext string, hubSecret []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	blob, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	aead, err := newWebPushAEAD(hubSecret)
	if err != nil {
		return "", err
	}
	if len(blob) < aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := blob[:aead.NonceSize()], blob[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt VAPID private: %w", err)
	}
	return string(pt), nil
}

func newWebPushAEAD(hubSecret []byte) (cipher.AEAD, error) {
	if len(hubSecret) == 0 {
		return nil, errors.New("hub secret unset")
	}
	h := sha256.New()
	h.Write([]byte("lumen/webpush/v1"))
	h.Write(hubSecret)
	block, err := aes.NewCipher(h.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
