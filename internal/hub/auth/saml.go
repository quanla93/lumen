// saml.go — SAML2 SSO config + lifecycle for the single-admin gate.
//
// Mirrors the OIDC module's shape (settings keys, LoadConfig,
// SaveConfig) so the Settings tab and the Login form see one
// consistent pattern. The flow + handlers live in saml_flow.go and
// saml_handlers.go; this file is config + keypair auto-generation
// + the typed expected_nameid list.
//
// RFC 0002 §"SP key + cert auto-generation": first PUT with
// enabled=true and no sp_cert row generates a 2048-bit RSA keypair
// + self-signed cert (CN = SP entity ID, 10y validity). The
// operator only pastes the IdP metadata XML — everything else is
// automatic on first save.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/quanla93/lumen/internal/hub/settings"
)

// Settings keys for SAML SSO. Stored in the generic settings table so
// runtime edits don't need a restart. The sp_private_key_enc value is
// encrypted via AES-GCM keyed off the hub session secret — see
// saml_crypto.go (label "lumen/saml/v1", distinct from OIDC's label
// so rotating LUMEN_HUB_SECRET doesn't break unrelated secrets).
const (
	SAMLKeyEnabled                = "saml.enabled"
	SAMLKeyIdPMetadataXML         = "saml.idp_metadata_xml"
	SAMLKeyIdPMetadataURL         = "saml.idp_metadata_url"
	SAMLKeySPEntityID             = "saml.sp_entity_id"
	SAMLKeyExpectedNameID         = "saml.expected_nameid"
	SAMLKeySPPrivateKeyEnc        = "saml.sp_private_key_enc"
	SAMLKeySPCert                 = "saml.sp_cert"
	SAMLKeyAllowedClockSkewSecs   = "saml.allowed_clock_skew_seconds"

	// SAMLAcsPath / SAMLLoginPath / SAMLMetadataPath are the public
	// routes the IdP redirects to / pulls from. Exposed as constants
	// so the docs and the handlers agree on a single source of truth.
	SAMLAcsPath        = "/api/auth/saml/acs"
	SAMLLoginPath      = "/api/auth/saml/login"
	SAMLMetadataPath   = "/api/auth/saml/metadata"

	// DefaultAllowedClockSkewSeconds is the tolerance on
	// NotOnOrAfter / NotBefore. 60s covers a 1-2 minute clock drift
	// between hub and IdP without enabling real replay.
	DefaultAllowedClockSkewSeconds = 60
)

// SAMLConfig is the resolved configuration the SAML flow operates on.
// Built by LoadSAMLConfig from the settings table + the hub secret;
// the SP private key is decrypted here so downstream code never
// touches the raw ciphertext.
type SAMLConfig struct {
	Enabled              bool
	IdPMetadataXML       string
	IdPMetadataURL       string
	SPEntityID           string
	ExpectedNameIDList   []string // comma-separated
	SPPrivateKeyPEM      string   // decrypted in memory; never logged
	SPCertPEM            string
	AllowedClockSkewSecs int
}

// LoadSAMLConfig reads the settings table and returns a SAMLConfig.
// An unset or malformed clock-skew value falls back to
// DefaultAllowedClockSkewSeconds so a fresh install never blocks the
// admin on the very first PUT.
func LoadSAMLConfig(ctx context.Context, db *sql.DB, hubSecret []byte) (SAMLConfig, error) {
	get := func(k string) string {
		v, _ := settings.Get(ctx, db, k)
		return v
	}

	cfg := SAMLConfig{
		Enabled:            get(SAMLKeyEnabled) == "true",
		IdPMetadataXML:     get(SAMLKeyIdPMetadataXML),
		IdPMetadataURL:     get(SAMLKeyIdPMetadataURL),
		SPEntityID:         get(SAMLKeySPEntityID),
		SPCertPEM:          get(SAMLKeySPCert),
		AllowedClockSkewSecs: DefaultAllowedClockSkewSeconds,
	}

	// ExpectedNameID: comma-separated per RFC Q4 proposed; trim
	// whitespace, drop empties. Match-any semantics — any one match
	// against the IdP-asserted NameID lets the user in.
	if raw := get(SAMLKeyExpectedNameID); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				cfg.ExpectedNameIDList = append(cfg.ExpectedNameIDList, part)
			}
		}
	}

	// Clock skew parse — silently fall back to default on garbage so
	// the operator doesn't have to debug a 502 from a typo.
	if raw := get(SAMLKeyAllowedClockSkewSecs); raw != "" {
		if n, err := time.ParseDuration(raw + "s"); err == nil && n > 0 {
			cfg.AllowedClockSkewSecs = int(n.Seconds())
		}
	}

	// Decrypt the SP private key on read. The on-disk row is empty
	// until auto-gen runs (see SaveSAMLConfig); an empty encrypted
	// value decodes to "" without error so the loader doesn't need
	// to know whether the key has been generated yet.
	if enc := get(SAMLKeySPPrivateKeyEnc); enc != "" {
		pem, err := DecryptSPKey(enc, hubSecret)
		if err != nil {
			return SAMLConfig{}, fmt.Errorf("SAML SP key: %w", err)
		}
		cfg.SPPrivateKeyPEM = pem
	}

	return cfg, nil
}

// SaveSAMLConfig persists the config. If enabled=true and no
// sp_cert is being saved, a 2048-bit RSA keypair + self-signed cert
// are auto-generated and the encrypted private key + cert PEM are
// written. This keeps the operator's only required input as
// "paste the IdP metadata XML" — everything else is automatic on
// first save.
//
// The auto-gen only fires when enabled is true; an admin who wants
// to clear config can PUT with enabled=false without triggering a
// fresh keypair.
func SaveSAMLConfig(ctx context.Context, db *sql.DB, hubSecret []byte, c SAMLConfig) error {
	if !c.Enabled {
		// Allowed to disable without a keypair present.
	} else {
		// Enabled-and-no-cert → auto-generate. If the operator is
		// updating an existing install we re-use the on-disk cert
		// (the form's PUT won't include a fresh cert, so the
		// existing row stays).
		if c.SPCertPEM == "" {
			existing, _ := settings.Get(ctx, db, SAMLKeySPCert)
			if existing == "" {
				pem, cert, err := generateSPKeypair(c.SPEntityID)
				if err != nil {
					return fmt.Errorf("SAML SP keypair: %w", err)
				}
				c.SPPrivateKeyPEM = pem
				c.SPCertPEM = cert
			}
		}
	}

	// Re-encrypt the private key for storage.
	enc, err := EncryptSPKey(c.SPPrivateKeyPEM, hubSecret)
	if err != nil {
		return fmt.Errorf("SAML SP key encrypt: %w", err)
	}

	// Round-trip the comma-separated expected_nameid list so the
	// operator's input is normalised (trim, dedupe, drop empties).
	var nameIDList string
	if len(c.ExpectedNameIDList) > 0 {
		seen := map[string]struct{}{}
		parts := make([]string, 0, len(c.ExpectedNameIDList))
		for _, v := range c.ExpectedNameIDList {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, dup := seen[v]; dup {
				continue
			}
			seen[v] = struct{}{}
			parts = append(parts, v)
		}
		nameIDList = strings.Join(parts, ",")
	}

	writes := map[string]string{
		SAMLKeyEnabled:              boolStr(c.Enabled),
		SAMLKeyIdPMetadataXML:       c.IdPMetadataXML,
		SAMLKeyIdPMetadataURL:       c.IdPMetadataURL,
		SAMLKeySPEntityID:           c.SPEntityID,
		SAMLKeyExpectedNameID:       nameIDList,
		SAMLKeySPPrivateKeyEnc:      enc,
		SAMLKeySPCert:               c.SPCertPEM,
		SAMLKeyAllowedClockSkewSecs: fmt.Sprintf("%d", c.AllowedClockSkewSecs),
	}
	for k, v := range writes {
		if err := settings.Set(ctx, db, k, v); err != nil {
			return fmt.Errorf("settings.Set %s: %w", k, err)
		}
	}
	return nil
}

// generateSPKeypair builds a 2048-bit RSA key + self-signed cert with
// CN = spEntityID, valid for 10 years. Returns the private key + cert
// as PEM blocks ready to encrypt and store.
func generateSPKeypair(spEntityID string) (keyPEM, certPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("rsa.GenerateKey: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("rand serial: %w", err)
	}
	notBefore := time.Now().UTC().Add(-time.Hour)
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour)
	cn := spEntityID
	if cn == "" {
		cn = "lumen-saml-sp"
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("x509.CreateCertificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("MarshalPKCS8PrivateKey: %w", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return keyPEM, certPEM, nil
}

// DefaultSPEntityID derives the SP entity ID from a public hub URL
// when the operator hasn't set one explicitly. Per RFC Q2 we use the
// metadata URL itself as the entity ID — that way the IdP sees a
// stable, self-referential identifier and the audience check matches
// trivially. hubPublicURL is e.g. "https://lumen.example.com" (no
// trailing slash).
func DefaultSPEntityID(hubPublicURL string) (string, error) {
	if hubPublicURL == "" {
		return "", errors.New("SAML: cannot default SP entity ID without a hub public URL")
	}
	u, err := url.Parse(hubPublicURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("SAML: hub public URL is not a valid URL: %q", hubPublicURL)
	}
	u.Path = SAMLMetadataPath
	return u.String(), nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
