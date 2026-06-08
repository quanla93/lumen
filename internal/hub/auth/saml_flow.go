// saml_flow.go — SAML2 SP wire-protocol layer on top of crewjam/saml.
//
// We use crewjam/saml's low-level types (ServiceProvider, ParseResponse)
// but skip its samlsp middleware — that middleware owns its own
// cookie-based session codec, which would conflict with Lumen's
// existing lumen_session JWT. Hand-rolling the four methods
// (LoginRedirect, HandleACS, Metadata, TestMetadata) keeps the
// session management on the existing path and lets the SAML flow
// return the validated NameID up to the handler, which mints
// lumen_session the same way the OIDC callback does.
//
// The single-admin gate is enforced here: a NameID that doesn't
// match any value in the expected_nameid list is rejected at ACS
// with a clear error, regardless of whether the IdP signature
// passed.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
)

// SAMLFlow owns the parsed crewjam/saml.ServiceProvider, cached so
// re-reads on every request don't reparse the IdP metadata. The
// cache is invalidated when the underlying settings fingerprint
// changes (cheap to compute on every request).
type SAMLFlow struct {
	DB         *sql.DB
	HubSecret  []byte
	HTTPClient *http.Client

	mu     sync.Mutex
	cached *cachedSAMLSP
}

type cachedSAMLSP struct {
	fingerprint string // hash of the (idp_xml, sp_cert, sp_key) tuple that built this SP
	sp          *saml.ServiceProvider
	idpMD       *saml.EntityDescriptor
	loadedAt    time.Time
}

// spFor returns a crewjam/saml.ServiceProvider for the current
// settings, rebuilding only when the underlying config fingerprint
// has changed. Returns nil + nil when SAML is disabled or required
// fields are unset — callers use that to 503.
func (f *SAMLFlow) spFor(ctx context.Context, hubPublicURL string) (*saml.ServiceProvider, *saml.EntityDescriptor, error) {
	cfg, err := LoadSAMLConfig(ctx, f.DB, f.HubSecret)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.Enabled || cfg.IdPMetadataXML == "" || cfg.SPPrivateKeyPEM == "" || cfg.SPCertPEM == "" {
		return nil, nil, nil
	}
	if len(cfg.ExpectedNameIDList) == 0 {
		return nil, nil, errors.New("SAML: at least one expected_nameid must be configured")
	}

	fp := fingerprint(cfg.IdPMetadataXML, cfg.SPPrivateKeyPEM, cfg.SPCertPEM)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cached != nil && f.cached.fingerprint == fp && time.Since(f.cached.loadedAt) < 10*time.Minute {
		return f.cached.sp, f.cached.idpMD, nil
	}

	idpMD, err := parseEntityDescriptor([]byte(cfg.IdPMetadataXML))
	if err != nil {
		return nil, nil, fmt.Errorf("SAML: parse IdP metadata: %w", err)
	}

	key, err := parseRSAPrivateKeyFromPEM(cfg.SPPrivateKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("SAML: parse SP private key: %w", err)
	}
	cert, err := parseCertificateFromPEM(cfg.SPCertPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("SAML: parse SP cert: %w", err)
	}

	hubURL, err := url.Parse(hubPublicURL)
	if err != nil || hubURL.Scheme == "" || hubURL.Host == "" {
		return nil, nil, fmt.Errorf("SAML: hub public URL is not a valid URL: %q", hubPublicURL)
	}

	// MetadataURL is the SP metadata endpoint. crewjam includes it
	// in the AuthnRequest + in the SP metadata XML, so set it to the
	// canonical SAMLMetadataPath.
	metadataURL := *hubURL
	metadataURL.Path = SAMLMetadataPath

	entityID := cfg.SPEntityID
	if entityID == "" {
		// RFC Q2: default to the metadata URL so the audience check
		// matches trivially.
		entityID = metadataURL.String()
	}

	acsURL := *hubURL
	acsURL.Path = SAMLAcsPath

	sp := &saml.ServiceProvider{
		EntityID:          entityID,
		Key:               key,
		Certificate:       cert,
		IDPMetadata:       idpMD,
		MetadataURL:       metadataURL,
		AcsURL:            acsURL,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
		// SignatureMethod controls whether the AuthnRequest is signed.
		// We don't sign by default; if the IdP demands it, the
		// operator can flip a future setting. Leaving the field
		// unset means unsigned.
	}

	f.cached = &cachedSAMLSP{
		fingerprint: fp,
		sp:          sp,
		idpMD:       idpMD,
		loadedAt:    time.Now(),
	}
	return sp, idpMD, nil
}

// LoginRedirect builds the AuthnRequest, sets a RelayState cookie so
// the ACS handler can verify InResponseTo, and returns the IdP's
// SingleSignOnService URL for the browser to follow.
//
// The state cookie is short-lived (5 min) and contains the request
// ID; ACS rejects SAMLResponses whose InResponseTo doesn't match a
// known recent request — replay protection.
func (f *SAMLFlow) LoginRedirect(ctx context.Context, w http.ResponseWriter, r *http.Request, hubPublicURL string) (string, error) {
	sp, _, err := f.spFor(ctx, hubPublicURL)
	if err != nil {
		return "", err
	}
	if sp == nil {
		return "", errors.New("SAML not configured")
	}

	// Build the AuthnRequest. crewjam wants the SSO URL + a binding.
	// We use the redirect binding (the standard SP-initiated flow).
	ssoURL := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)
	if ssoURL == "" {
		// Fall back to the POST binding for IdPs that disable
		// the redirect binding.
		ssoURL = sp.GetSSOBindingLocation(saml.HTTPPostBinding)
	}
	if ssoURL == "" {
		return "", errors.New("SAML: IdP metadata has no HTTP redirect or POST SSO binding")
	}

	req, err := sp.MakeAuthenticationRequest(ssoURL, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return "", fmt.Errorf("SAML: build AuthnRequest: %w", err)
	}

	// Stash the request ID in a short-lived cookie so ACS can verify
	// InResponseTo. The crewjam CookieRequestTracker does the same
	// under the hood; we hand-roll it because we're not using the
	// cookie session codec.
	http.SetCookie(w, &http.Cookie{
		Name:     "lumen_saml_relay",
		Value:    req.ID,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Build the URL the browser should follow. crewjam's
	// AuthnRequest.Redirect returns a *url.URL with SAMLRequest +
	// optional RelayState + signature params (if signing is on)
	// already encoded.
	redirectURL, err := req.Redirect("", sp)
	if err != nil {
		return "", fmt.Errorf("SAML: encode redirect: %w", err)
	}
	return redirectURL.String(), nil
}

// HandleACS validates a SAMLResponse POSTed by the IdP. On success
// it returns the validated NameID; the handler then mints the
// lumen_session cookie the same way the OIDC callback does.
//
// Validation order: XML signature (crewjam default) → InResponseTo
// matches our relay cookie → conditions (NotOnOrAfter, Audience,
// NotBefore) → NameID membership in the expected list.
func (f *SAMLFlow) HandleACS(ctx context.Context, w http.ResponseWriter, r *http.Request, hubPublicURL string) (string, error) {
	cfg, err := LoadSAMLConfig(ctx, f.DB, f.HubSecret)
	if err != nil {
		return "", err
	}
	sp, _, err := f.spFor(ctx, hubPublicURL)
	if err != nil {
		return "", err
	}
	if sp == nil {
		return "", errors.New("SAML not configured")
	}

	// SAMLResponse is form-encoded in the POST body.
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("SAML: parse form: %w", err)
	}
	samlResponseB64 := r.PostFormValue("SAMLResponse")
	if samlResponseB64 == "" {
		return "", errors.New("SAML: missing SAMLResponse in POST")
	}
	raw, err := base64.StdEncoding.DecodeString(samlResponseB64)
	if err != nil {
		return "", fmt.Errorf("SAML: base64 decode: %w", err)
	}
	// crewjam's ParseResponse expects the bytes in the request body;
	// splice them in via a request whose body is a Reader of `raw`.
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	r.ContentLength = int64(len(raw))

	// InResponseTo must match the RelayState cookie we set at login
	// time. crewjam verifies this against possibleRequestIDs.
	cookie, err := r.Cookie("lumen_saml_relay")
	if err != nil {
		return "", fmt.Errorf("SAML: missing relay state cookie: %w", err)
	}
	possibleIDs := []string{cookie.Value}

	// Parse + validate the response. ParseResponse does the
	// signature check + InResponseTo + condition checks (with the
	// library's default clock skew).
	assertion, err := sp.ParseResponse(r, possibleIDs)
	if err != nil {
		return "", fmt.Errorf("SAML: validate response: %w", err)
	}

	if assertion.Subject == nil || assertion.Subject.NameID == nil {
		return "", errors.New("SAML: response missing NameID")
	}
	nameID := assertion.Subject.NameID.Value

	// Single-admin gate (RFC §"Scope", RFC Q4 proposed comma-
	// separated intersect-any).
	if !nameIDAllowed(nameID, cfg.ExpectedNameIDList) {
		return "", fmt.Errorf("SAML: NameID %q not in expected_nameid list", nameID)
	}

	// Re-check the time window with the operator's configured skew
	// so the setting is honored even if crewjam uses a different
	// default. crewjam already enforced NotOnOrAfter with its own
	// skew by the time we get here; this is a second opinion.
	if !checkConditionsWindow(assertion.Conditions, cfg.AllowedClockSkewSecs) {
		return "", errors.New("SAML: assertion outside allowed time window")
	}

	// Clear the relay cookie so the same browser can't reuse it on
	// a later SAMLResponse (the check above already matched the
	// cookie's value; clearing the cookie prevents replays).
	if w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "lumen_saml_relay",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	return nameID, nil
}

// Metadata returns the SP metadata XML for the configured hub. The
// IdP ingests this so it knows the hub's entity ID, ACS URL, and
// the SP cert (for signed response verification).
func (f *SAMLFlow) Metadata(ctx context.Context, hubPublicURL string) ([]byte, error) {
	sp, _, err := f.spFor(ctx, hubPublicURL)
	if err != nil {
		return nil, err
	}
	if sp == nil {
		return nil, errors.New("SAML not configured")
	}
	// crewjam's Metadata() returns the EntityDescriptor as a Go
	// struct; the saml package has MarshalXML helpers we'll use
	// via the schema's xml marshaller. The simplest portable path
	// is to feed a tiny template that wraps the EntityDescriptor
	// in the canonical <EntityDescriptor> root with the namespaces
	// a real IdP expects.
	ed := sp.Metadata()
	if ed == nil {
		return nil, errors.New("SAML: ServiceProvider.Metadata returned nil")
	}
	// Use encoding/xml via the saml package's built-in
	// MarshalIndent on the EntityDescriptor's XMLName.
	return marshalEntityDescriptor(ed)
}

// TestMetadata fetches (or parses) IdP metadata and returns the
// discovered SSO URL + IdP entity ID so the operator can confirm
// the input is sane before saving.
//
// The handler picks which of (xmlPaste, metadataURL) to pass; we
// reject the call if both are empty.
func (f *SAMLFlow) TestMetadata(ctx context.Context, xmlPaste string, metadataURL string) (ssoURL string, idpEntityID string, err error) {
	var raw []byte
	if xmlPaste != "" {
		raw = []byte(xmlPaste)
	} else if metadataURL != "" {
		u, perr := url.Parse(metadataURL)
		if perr != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", "", fmt.Errorf("SAML: metadata URL must be http(s): %q", metadataURL)
		}
		cli := f.HTTPClient
		if cli == nil {
			cli = &http.Client{Timeout: 10 * time.Second}
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", metadataURL, nil)
		resp, gerr := cli.Do(req)
		if gerr != nil {
			return "", "", fmt.Errorf("SAML: fetch metadata: %w", gerr)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return "", "", fmt.Errorf("SAML: metadata fetch returned %d", resp.StatusCode)
		}
		raw, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("SAML: read metadata: %w", err)
		}
	} else {
		return "", "", errors.New("SAML: test-metadata requires xml or url")
	}

	md, perr := parseEntityDescriptor(raw)
	if perr != nil {
		return "", "", fmt.Errorf("SAML: parse metadata: %w", perr)
	}
	for _, idpSSO := range md.IDPSSODescriptors {
		for _, ss := range idpSSO.SingleSignOnServices {
			if ss.Binding == saml.HTTPRedirectBinding || ss.Binding == saml.HTTPPostBinding {
				ssoURL = ss.Location
				break
			}
		}
		if ssoURL != "" {
			break
		}
	}
	if ssoURL == "" {
		return "", "", errors.New("SAML: metadata has no SingleSignOnService with a redirect or POST binding")
	}
	idpEntityID = md.EntityID
	return ssoURL, idpEntityID, nil
}

// nameIDAllowed is the single-admin gate. RFC Q4 proposed comma-
// separated intersect-any semantics; we honour that here. Equality
// is case-insensitive (email local-parts are commonly case-insensitive
// at the IdP).
func nameIDAllowed(nameID string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, nameID) {
			return true
		}
	}
	return false
}

// checkConditionsWindow is a defensive double-check beyond what
// crewjam does by default. crewjam's ParseResponse enforces
// NotOnOrAfter with the standard library clock skew; we add a
// secondary check using the operator's configured skew so a
// misconfigured IdP can't sneak an expired assertion through.
func checkConditionsWindow(c *saml.Conditions, skewSecs int) bool {
	if c == nil {
		return true
	}
	now := time.Now()
	skew := time.Duration(skewSecs) * time.Second
	// saml.Conditions.NotBefore / NotOnOrAfter are time.Time, not
	// strings. The zero value means "no constraint".
	if !c.NotBefore.IsZero() && now.Add(skew).Before(c.NotBefore) {
		return false
	}
	if !c.NotOnOrAfter.IsZero() && now.Add(-skew).After(c.NotOnOrAfter) {
		return false
	}
	return true
}

// fingerprint is a cheap hash of the inputs that affect SP/IdP
// parsing. A change to any of them forces a reparse.
func fingerprint(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// parseRSAPrivateKeyFromPEM accepts either PKCS#1 ("RSA PRIVATE KEY")
// or PKCS#8 ("PRIVATE KEY") and returns the *rsa.PrivateKey.
func parseRSAPrivateKeyFromPEM(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("SAML: SP key is not valid PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("SAML: SP key not PKCS#1 or PKCS#8: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("SAML: SP key is not an RSA key")
	}
	return rsaKey, nil
}

// parseCertificateFromPEM parses a single CERTIFICATE PEM block.
func parseCertificateFromPEM(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("SAML: SP cert is not valid PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

// Used to keep the io / rand imports honest when saml.spFor rebuilds.
var _ = io.Discard
var _ = rand.Reader
var _ = pkix.Name{}
