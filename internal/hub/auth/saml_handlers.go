// saml_handlers.go — HTTP handlers for the SAML feature.
//
// Endpoints (per RFC 0002 §"Endpoints"):
//   GET  /api/auth/saml/login      — public, 302 to IdP SSO URL
//   POST /api/auth/saml/acs        — public, IdP POSTs SAMLResponse here
//   GET  /api/auth/saml/metadata   — public, returns SP metadata XML
//   GET  /api/settings/saml        — session, returns config (keypair → has_sp_keypair)
//   PUT  /api/settings/saml        — session, saves config (auto-generates SP keypair)
//   POST /api/settings/saml/test-metadata — session, parses paste or fetches URL
//
// On successful ACS the handler mints the same lumen_session JWT
// the OIDC callback does — same single-admin gate, same cookie
// attributes, same response shape.

package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/quanla93/lumen/internal/hub/settings"
)

// SAMLSettingsView is the wire shape for GET/PUT /api/settings/saml.
// The SP private key never round-trips back; has_sp_keypair tells
// the UI "already set" vs "not yet generated".
type SAMLSettingsView struct {
	Enabled              bool     `json:"enabled"`
	IdPMetadataXML       string   `json:"idp_metadata_xml,omitempty"`
	IdPMetadataURL       string   `json:"idp_metadata_url,omitempty"`
	SPEntityID           string   `json:"sp_entity_id,omitempty"`
	ExpectedNameID       string   `json:"expected_nameid,omitempty"`
	HasSPKeypair         bool     `json:"has_sp_keypair"`
	SPCertPEM            string   `json:"sp_cert,omitempty"`
	AllowedClockSkewSecs int      `json:"allowed_clock_skew_seconds"`
	DiscoveredSSOURL     string   `json:"discovered_sso_url,omitempty"`
	DiscoveredEntityID   string   `json:"discovered_entity_id,omitempty"`
}

// samlSettingsGet — GET /api/settings/saml
func (h *Handlers) SAMLSettingsGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := LoadSAMLConfig(ctx, h.DB, h.Secret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	writeJSON(w, http.StatusOK, SAMLSettingsView{
		Enabled:              cfg.Enabled,
		IdPMetadataXML:       cfg.IdPMetadataXML,
		IdPMetadataURL:       cfg.IdPMetadataURL,
		SPEntityID:           cfg.SPEntityID,
		ExpectedNameID:       joinCommaList(cfg.ExpectedNameIDList),
		HasSPKeypair:         cfg.SPPrivateKeyPEM != "",
		SPCertPEM:            cfg.SPCertPEM,
		AllowedClockSkewSecs: cfg.AllowedClockSkewSecs,
	})
}

// samlSettingsPut — PUT /api/settings/saml
//
// Saves the config. When enabled=true and no sp_cert is in the
// incoming body, SaveSAMLConfig auto-generates the SP keypair +
// self-signed cert (per RFC §"SP key + cert auto-generation"). The
// operator never has to generate a key by hand.
func (h *Handlers) SAMLSettingsPut(w http.ResponseWriter, r *http.Request) {
	var in SAMLSettingsView
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	expected, err := splitCommaList(in.ExpectedNameID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "expected_nameid: "+err.Error())
		return
	}

	// If the operator is editing an existing config and didn't
	// include a sp_cert in the body, preserve the on-disk value
	// so we don't trigger a fresh auto-gen on every save.
	spCert := in.SPCertPEM
	if spCert == "" {
		existing, _ := settings.Get(r.Context(), h.DB, SAMLKeySPCert)
		spCert = existing
	}

	cfg := SAMLConfig{
		Enabled:              in.Enabled,
		IdPMetadataXML:       in.IdPMetadataXML,
		IdPMetadataURL:       in.IdPMetadataURL,
		SPEntityID:           in.SPEntityID,
		ExpectedNameIDList:   expected,
		SPCertPEM:            spCert,
		AllowedClockSkewSecs: in.AllowedClockSkewSecs,
	}
	if cfg.AllowedClockSkewSecs <= 0 {
		cfg.AllowedClockSkewSecs = DefaultAllowedClockSkewSeconds
	}

	if err := SaveSAMLConfig(r.Context(), h.DB, h.Secret, cfg); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "save: "+err.Error())
		return
	}

	// Re-read so the response shows the auto-generated cert when
	// applicable. Echo the new state back the same way the GET
	// handler does.
	saved, err := LoadSAMLConfig(r.Context(), h.DB, h.Secret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "post-save read: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, SAMLSettingsView{
		Enabled:              saved.Enabled,
		IdPMetadataXML:       saved.IdPMetadataXML,
		IdPMetadataURL:       saved.IdPMetadataURL,
		SPEntityID:           saved.SPEntityID,
		ExpectedNameID:       joinCommaList(saved.ExpectedNameIDList),
		HasSPKeypair:         saved.SPPrivateKeyPEM != "",
		SPCertPEM:            saved.SPCertPEM,
		AllowedClockSkewSecs: saved.AllowedClockSkewSecs,
	})
}

// samlTestMetadata — POST /api/settings/saml/test-metadata
//
// Body: {xml?: string, url?: string}. Returns
// {ok, sso_url, idp_entity_id, error?} so the UI can show "we
// found this IdP entity at this URL — does that look right?"
// before the operator hits Save.
func (h *Handlers) SAMLTestMetadata(w http.ResponseWriter, r *http.Request) {
	var in struct {
		XML string `json:"xml"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if h.SAML == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "SAML not configured")
		return
	}
	ssoURL, idpEntity, err := h.SAML.TestMetadata(r.Context(), in.XML, in.URL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"sso_url":        ssoURL,
		"idp_entity_id":  idpEntity,
	})
}

// LoginSAML — GET /api/auth/saml/login
//
// Sets a short-lived RelayState cookie with the AuthnRequest ID, then
// 302s the browser to the IdP's SSO URL with SAMLRequest encoded in
// the query string. The cookie is the InResponseTo anchor the ACS
// handler checks.
func (h *Handlers) LoginSAML(w http.ResponseWriter, r *http.Request) {
	if h.SAML == nil {
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New("SAML not configured")), http.StatusSeeOther)
		return
	}
	hubPublic := h.HubPublicURL()
	if hubPublic == "" {
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New("LUMEN_HUB_PUBLIC_URL not set")), http.StatusSeeOther)
		return
	}
	redirectURL, err := h.SAML.LoginRedirect(r.Context(), w, r, hubPublic)
	if err != nil {
		h.Logger.Warn("saml login redirect failed", "err", err)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// ACSHandler — POST /api/auth/saml/acs
//
// IdP POSTs SAMLResponse here. Validate → extract NameID → mint the
// same lumen_session JWT password login + OIDC callback do → 302 to /.
func (h *Handlers) ACSHandler(w http.ResponseWriter, r *http.Request) {
	if h.SAML == nil {
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New("SAML not configured")), http.StatusSeeOther)
		return
	}
	hubPublic := h.HubPublicURL()
	if hubPublic == "" {
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New("LUMEN_HUB_PUBLIC_URL not set")), http.StatusSeeOther)
		return
	}
	nameID, err := h.SAML.HandleACS(r.Context(), w, r, hubPublic)
	if err != nil {
		h.Logger.Warn("saml acs failed", "err", err)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(err), http.StatusSeeOther)
		return
	}
	// Mint the session cookie. Look up the user by username == nameID
	// (we bind SAML to the existing single admin row; the expected
	// list is the operator's authoritative allowlist).
	uid, lookupErr := lookupUserIDByUsername(r.Context(), h.DB, nameID)
	if lookupErr != nil || uid == 0 {
		// Either no matching admin row, or more than one (the gate
		// already proved the NameID is expected). Single-admin
		// means there's exactly one row; the rest is operator error.
		h.Logger.Warn("saml acs: NameID matched expected list but no user row", "name_id", nameID, "lookup_err", lookupErr)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New("SAML: NameID has no matching admin")), http.StatusSeeOther)
		return
	}
	if err := h.issueAndSetCookie(w, r, uid); err != nil {
		h.Logger.Warn("saml acs: mint session failed", "err", err)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// MetadataSAML — GET /api/auth/saml/metadata
//
// Returns the SP metadata XML for the IdP to ingest.
func (h *Handlers) MetadataSAML(w http.ResponseWriter, r *http.Request) {
	if h.SAML == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "SAML not configured")
		return
	}
	hubPublic := h.HubPublicURL()
	if hubPublic == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "LUMEN_HUB_PUBLIC_URL not set")
		return
	}
	md, err := h.SAML.Metadata(r.Context(), hubPublic)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "metadata: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(md)
}

// joinCommaList + splitCommaList are tiny normalisers for the
// expected_nameid wire field. RFC Q4 lets the operator paste a
// comma-separated list; we trim + dedupe + drop empties on save
// and join with commas on read so the UI round-trips cleanly.
func joinCommaList(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

func splitCommaList(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	parts := []string{}
	seen := map[string]struct{}{}
	curr := ""
	for _, r := range s {
		if r == ',' {
			addIfNew(&parts, seen, curr)
			curr = ""
			continue
		}
		curr += string(r)
	}
	addIfNew(&parts, seen, curr)
	if len(parts) == 0 {
		return nil, errors.New("no valid entries")
	}
	return parts, nil
}

func addIfNew(out *[]string, seen map[string]struct{}, v string) {
	v = trimSpace(v)
	if v == "" {
		return
	}
	if _, dup := seen[v]; dup {
		return
	}
	seen[v] = struct{}{}
	*out = append(*out, v)
}

func trimSpace(s string) string {
	// Inline to avoid pulling in strings just for this; reset to
	// the stdlib version if it ever gets used in more places.
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
