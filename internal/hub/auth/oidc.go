package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/quanla93/lumen/internal/hub/settings"
	"golang.org/x/oauth2"
)

// Settings keys for OIDC SSO. Stored in the generic settings table so
// runtime config edits don't need a restart. client_secret is encrypted
// via AES-GCM keyed off the hub session secret — see crypto.go.
const (
	OIDCKeyEnabled         = "oidc.enabled"
	OIDCKeyIssuer          = "oidc.issuer"
	OIDCKeyClientID        = "oidc.client_id"
	OIDCKeyClientSecretEnc = "oidc.client_secret_enc"
	OIDCKeyScopes          = "oidc.scopes"
	OIDCKeyExpectedEmail   = "oidc.expected_email"
)

const defaultOIDCScopes = "openid email profile"

// OIDC redirect path on the hub — must match what the operator registers
// in their IdP. Exposed as a constant so the docs and the handler agree.
const OIDCCallbackPath = "/api/auth/oidc/callback"

// State cookie that carries the OAuth state + ID-token nonce across the
// browser redirect. Short-lived; cleared on callback success or any error.
const (
	oidcStateCookie = "lumen_oidc_state"
	oidcStateTTL    = 5 * time.Minute
)

// OIDCConfig is the decoded settings, ready for either the runtime flow
// or display in the Settings UI. ClientSecret is empty when read for
// display (operator never sees the existing secret in plaintext on the
// wire); it's only populated when the runtime flow decrypts it.
type OIDCConfig struct {
	Enabled       bool
	Issuer        string
	ClientID      string
	ClientSecret  string
	Scopes        string
	ExpectedEmail string
}

// LoadOIDCConfig pulls all OIDC settings from the database. Pass
// decryptSecret=true to populate ClientSecret; the UI path uses false.
func LoadOIDCConfig(ctx context.Context, db *sql.DB, hubSecret []byte, decryptSecret bool) (OIDCConfig, error) {
	cfg := OIDCConfig{Scopes: defaultOIDCScopes}
	if v, _ := settings.Get(ctx, db, OIDCKeyEnabled); v == "true" {
		cfg.Enabled = true
	}
	if v, _ := settings.Get(ctx, db, OIDCKeyIssuer); v != "" {
		cfg.Issuer = strings.TrimRight(v, "/")
	}
	if v, _ := settings.Get(ctx, db, OIDCKeyClientID); v != "" {
		cfg.ClientID = v
	}
	if v, _ := settings.Get(ctx, db, OIDCKeyScopes); v != "" {
		cfg.Scopes = v
	}
	if v, _ := settings.Get(ctx, db, OIDCKeyExpectedEmail); v != "" {
		cfg.ExpectedEmail = strings.ToLower(strings.TrimSpace(v))
	}
	if decryptSecret {
		enc, _ := settings.Get(ctx, db, OIDCKeyClientSecretEnc)
		if enc != "" {
			sec, err := DecryptSecret(enc, hubSecret)
			if err != nil {
				return cfg, fmt.Errorf("decrypt OIDC client secret: %w", err)
			}
			cfg.ClientSecret = sec
		}
	}
	return cfg, nil
}

// SaveOIDCConfig persists the settings. An empty ClientSecret leaves the
// existing one untouched, so the UI doesn't have to retransmit it on
// every save.
func SaveOIDCConfig(ctx context.Context, db *sql.DB, hubSecret []byte, cfg OIDCConfig) error {
	if err := settings.Set(ctx, db, OIDCKeyEnabled, strconv.FormatBool(cfg.Enabled)); err != nil {
		return err
	}
	if err := settings.Set(ctx, db, OIDCKeyIssuer, strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")); err != nil {
		return err
	}
	if err := settings.Set(ctx, db, OIDCKeyClientID, strings.TrimSpace(cfg.ClientID)); err != nil {
		return err
	}
	if err := settings.Set(ctx, db, OIDCKeyScopes, strings.TrimSpace(cfg.Scopes)); err != nil {
		return err
	}
	if err := settings.Set(ctx, db, OIDCKeyExpectedEmail, strings.ToLower(strings.TrimSpace(cfg.ExpectedEmail))); err != nil {
		return err
	}
	if cfg.ClientSecret != "" {
		enc, err := EncryptSecret(cfg.ClientSecret, hubSecret)
		if err != nil {
			return err
		}
		if err := settings.Set(ctx, db, OIDCKeyClientSecretEnc, enc); err != nil {
			return err
		}
	}
	return nil
}

// OIDCFlow is the runtime side: caches the discovered provider + verifier
// keyed by issuer so we don't refetch the OpenID config + JWKS on every
// login. Cache is invalidated when the operator saves a new issuer.
type OIDCFlow struct {
	DB         *sql.DB
	HubSecret  []byte
	HTTPClient *http.Client

	mu       sync.Mutex
	cached   *cachedOIDCProvider
	cachedAt time.Time
}

type cachedOIDCProvider struct {
	issuer       string
	clientID     string
	clientSecret string
	scopes       string
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
}

// providerFor returns a ready oidc.Provider + verifier for the current
// settings, fetching discovery + JWKS on first call (or after invalidate).
// Returns false if OIDC is disabled or required fields are unset.
func (f *OIDCFlow) providerFor(ctx context.Context) (*cachedOIDCProvider, OIDCConfig, bool, error) {
	cfg, err := LoadOIDCConfig(ctx, f.DB, f.HubSecret, true)
	if err != nil {
		return nil, cfg, false, err
	}
	if !cfg.Enabled || cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, cfg, false, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.cached; c != nil &&
		c.issuer == cfg.Issuer &&
		c.clientID == cfg.ClientID &&
		c.clientSecret == cfg.ClientSecret &&
		c.scopes == cfg.Scopes &&
		time.Since(f.cachedAt) < time.Hour {
		return c, cfg, true, nil
	}

	disc := oidc.NewProvider
	provCtx := oidc.ClientContext(ctx, f.HTTPClient)
	provider, err := disc(provCtx, cfg.Issuer)
	if err != nil {
		return nil, cfg, false, fmt.Errorf("oidc discovery for %q: %w", cfg.Issuer, err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	c := &cachedOIDCProvider{
		issuer:       cfg.Issuer,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		scopes:       cfg.Scopes,
		provider:     provider,
		verifier:     verifier,
	}
	f.cached = c
	f.cachedAt = time.Now()
	return c, cfg, true, nil
}

// Invalidate forces the next flow call to refetch discovery + JWKS.
// Called after Settings → SSO save so a key/issuer change takes effect
// without a hub restart.
func (f *OIDCFlow) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached = nil
}

// LoginRedirect drives the start of the auth-code flow. Generates a
// random state + nonce, packs both into a short-lived signed cookie, and
// returns the issuer's authorization URL for the caller to 302 to.
func (f *OIDCFlow) LoginRedirect(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, error) {
	cp, _, ok, err := f.providerFor(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("OIDC is not configured")
	}
	state, nonce, err := newStateNonce()
	if err != nil {
		return "", err
	}
	setStateCookie(w, r, state+"|"+nonce, f.HubSecret)

	oauthCfg := f.oauth2Config(cp, hubURLFromRequest(r))
	url := oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce))
	return url, nil
}

// HandleCallback validates the IdP's redirect, exchanges code for tokens,
// verifies the ID token, checks the email matches the operator's
// allow-list value (OIDCKeyExpectedEmail), and returns the matched email
// so the caller can mint a session. Returns ("", err) on any mismatch.
func (f *OIDCFlow) HandleCallback(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, error) {
	cp, cfg, ok, err := f.providerFor(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("OIDC is not configured")
	}

	stateBlob, err := readStateCookie(r, f.HubSecret)
	clearStateCookie(w, r)
	if err != nil {
		return "", fmt.Errorf("state cookie: %w", err)
	}
	parts := strings.SplitN(stateBlob, "|", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed state cookie")
	}
	wantState, wantNonce := parts[0], parts[1]

	if r.URL.Query().Get("state") != wantState {
		return "", errors.New("state mismatch")
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		return "", errors.New("missing authorization code")
	}

	oauthCfg := f.oauth2Config(cp, hubURLFromRequest(r))
	exchangeCtx := oidc.ClientContext(ctx, f.HTTPClient)
	tok, err := oauthCfg.Exchange(exchangeCtx, code)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", errors.New("id_token missing from provider response")
	}
	idTok, err := cp.verifier.Verify(ctx, rawID)
	if err != nil {
		return "", fmt.Errorf("id_token verify: %w", err)
	}
	if idTok.Nonce != wantNonce {
		return "", errors.New("nonce mismatch")
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Sub           string `json:"sub"`
	}
	if err := idTok.Claims(&claims); err != nil {
		return "", fmt.Errorf("claims decode: %w", err)
	}
	gotEmail := strings.ToLower(strings.TrimSpace(claims.Email))
	if gotEmail == "" {
		return "", errors.New("provider returned no email; ensure scope 'email' is granted")
	}
	if cfg.ExpectedEmail == "" {
		return "", errors.New("settings → SSO has no expected email configured")
	}
	if gotEmail != cfg.ExpectedEmail {
		return "", fmt.Errorf("OIDC email %q does not match expected %q", gotEmail, cfg.ExpectedEmail)
	}
	return gotEmail, nil
}

func (f *OIDCFlow) oauth2Config(cp *cachedOIDCProvider, hubBaseURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cp.clientID,
		ClientSecret: cp.clientSecret,
		Endpoint:     cp.provider.Endpoint(),
		RedirectURL:  strings.TrimRight(hubBaseURL, "/") + OIDCCallbackPath,
		Scopes:       splitScopes(cp.scopes),
	}
}

// TestDiscovery fetches the issuer's discovery doc without going through
// the auth flow. Lets the Settings UI surface "discovery OK" vs
// "unreachable / 404 / bad config" before the operator hits Save.
func (f *OIDCFlow) TestDiscovery(ctx context.Context, issuer string) error {
	if issuer == "" {
		return errors.New("issuer is empty")
	}
	provCtx := oidc.ClientContext(ctx, f.HTTPClient)
	_, err := oidc.NewProvider(provCtx, strings.TrimRight(issuer, "/"))
	return err
}

// --- helpers ----------------------------------------------------------

func splitScopes(s string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = strings.Fields(defaultOIDCScopes)
	}
	return out
}

func newStateNonce() (string, string, error) {
	mk := func() (string, error) {
		var b [24]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(b[:]), nil
	}
	state, err := mk()
	if err != nil {
		return "", "", err
	}
	nonce, err := mk()
	if err != nil {
		return "", "", err
	}
	return state, nonce, nil
}

// setStateCookie encrypts the state||nonce blob so the IdP can't replay
// or forge it back through the browser. AES-GCM keyed off the hub secret;
// 5-minute TTL.
func setStateCookie(w http.ResponseWriter, r *http.Request, blob string, hubSecret []byte) {
	enc, err := EncryptSecret(blob, hubSecret)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    enc,
		Path:     "/",
		Secure:   isHTTPS(r),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(oidcStateTTL),
		MaxAge:   int(oidcStateTTL.Seconds()),
	})
}

func readStateCookie(r *http.Request, hubSecret []byte) (string, error) {
	c, err := r.Cookie(oidcStateCookie)
	if err != nil {
		return "", err
	}
	return DecryptSecret(c.Value, hubSecret)
}

func clearStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    "",
		Path:     "/",
		Secure:   isHTTPS(r),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// hubURLFromRequest derives the hub's public URL from the inbound
// request, honouring reverse-proxy headers. Used to build the OIDC
// redirect_uri so it matches what the operator registered with the IdP
// without an additional LUMEN_HUB_PUBLIC_URL env var.
func hubURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		scheme = xfp
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return scheme + "://" + strings.TrimRight(host, "/")
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
