package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
)

// TestNameIDAllowed is the heart of the single-admin gate. RFC Q4
// proposed comma-separated intersect-any (case-insensitive). We
// use UPN-style strings ("user", "ADMIN") rather than full emails
// here so the test inputs survive any terminal-side obfuscation
// that mangles email addresses — the gate logic is the same
// regardless of the NameID format.
func TestNameIDAllowed(t *testing.T) {
	allowed := []string{"alice", "bob"}
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"exact match (lowercase)", "alice", true},
		{"case-insensitive match", "ALICE", true},
		{"unknown user", "carol", false},
		{"empty", "", false},
		{"trailing whitespace does not sneak through", "alice ", false},
		{"prefix of allowed value is not allowed", "ali", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nameIDAllowed(c.in, allowed); got != c.want {
				t.Errorf("nameIDAllowed(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestNameIDAllowed_EmptyList rejects everything — protects against
// a misconfiguration where the operator enabled SAML before adding
// any expected_nameid values.
func TestNameIDAllowed_EmptyList(t *testing.T) {
	if nameIDAllowed("alice", nil) {
		t.Error("empty allowed list must reject any NameID")
	}
	if nameIDAllowed("alice", []string{}) {
		t.Error("empty allowed list must reject any NameID")
	}
}

// TestCheckConditionsWindow covers the secondary time-window check
// the SAML flow does after crewjam's own parse. We exercise the
// boundaries relative to the operator-configured skew.
func TestCheckConditionsWindow(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name   string
		conds  *saml.Conditions
		skew   int
		wantOK bool
	}{
		{"nil conditions pass", nil, 60, true},
		{"notBefore in past passes", &saml.Conditions{NotBefore: now.Add(-time.Hour), NotOnOrAfter: now.Add(time.Hour)}, 60, true},
		{"notBefore in future fails", &saml.Conditions{NotBefore: now.Add(time.Hour)}, 60, false},
		{"notOnOrAfter in past fails", &saml.Conditions{NotOnOrAfter: now.Add(-time.Hour)}, 60, false},
		{"notOnOrAfter just past passes with skew", &saml.Conditions{NotOnOrAfter: now.Add(-30 * time.Second)}, 60, true},
		{"notOnOrAfter further past fails with skew", &saml.Conditions{NotOnOrAfter: now.Add(-2 * time.Minute)}, 60, false},
		{"notBefore just future passes with skew", &saml.Conditions{NotBefore: now.Add(30 * time.Second)}, 60, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkConditionsWindow(c.conds, c.skew); got != c.wantOK {
				t.Errorf("checkConditionsWindow(%+v, %d) = %v, want %v", c.conds, c.skew, got, c.wantOK)
			}
		})
	}
}

// TestParseEntityDescriptor_OktaShape confirms the parser tolerates a
// typical IdP metadata document. The fixture is a stripped version
// of the Okta classic metadata shape — namespaces in
// <EntityDescriptor> root, one IDPSSODescriptor with one
// SingleSignOnService.
func TestParseEntityDescriptor_OktaShape(t *testing.T) {
	const okta = `<?xml version="1.0" encoding="UTF-8"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="http://www.okta.com/exk1q7m4iyP9a2xI0h7">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>MIIDazCCAlOgAwIBAgIUJj7q</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://example.okta.com/app/lumen/exk1q7m4iyP9a2xI0h7/sso/saml"/>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://example.okta.com/app/lumen/exk1q7m4iyP9a2xI0h7/sso/saml"/>
  </IDPSSODescriptor>
</EntityDescriptor>`
	md, err := parseEntityDescriptor([]byte(okta))
	if err != nil {
		t.Fatalf("parseEntityDescriptor: %v", err)
	}
	if md.EntityID != "http://www.okta.com/exk1q7m4iyP9a2xI0h7" {
		t.Errorf("EntityID = %q, want okta URL", md.EntityID)
	}
	if len(md.IDPSSODescriptors) != 1 {
		t.Fatalf("IDPSSODescriptors len = %d, want 1", len(md.IDPSSODescriptors))
	}
	ssos := md.IDPSSODescriptors[0].SingleSignOnServices
	if len(ssos) != 2 {
		t.Fatalf("SingleSignOnServices len = %d, want 2", len(ssos))
	}
	if !strings.HasPrefix(ssos[0].Location, "https://example.okta.com/") {
		t.Errorf("SSO[0] Location = %q, want okta URL", ssos[0].Location)
	}
}

// TestParseEntityDescriptor_FederationWrapper covers the
// <EntitiesDescriptor> case (federation bundles) — common for
// inCommon and other edu-federations.
func TestParseEntityDescriptor_FederationWrapper(t *testing.T) {
	const fed = `<?xml version="1.0" encoding="UTF-8"?>
<EntitiesDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata">
  <EntityDescriptor entityID="https://idp1.example/idp">
    <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
      <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp1.example/sso"/>
    </IDPSSODescriptor>
  </EntityDescriptor>
  <EntityDescriptor entityID="https://idp2.example/idp">
    <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
      <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp2.example/sso"/>
    </IDPSSODescriptor>
  </EntityDescriptor>
</EntitiesDescriptor>`
	md, err := parseEntityDescriptor([]byte(fed))
	if err != nil {
		t.Fatalf("parseEntityDescriptor: %v", err)
	}
	if md.EntityID != "https://idp1.example/idp" {
		t.Errorf("EntityID = %q, want first idp from federation", md.EntityID)
	}
}

// TestParseEntityDescriptor_RejectsGarbage covers the failure path
// so a typo in the paste box surfaces a clear error rather than
// crashing the handler.
func TestParseEntityDescriptor_RejectsGarbage(t *testing.T) {
	for _, bad := range []string{
		"",
		"<not-xml-at-all",
		"<?xml version='1.0'?><NotARealEntity/>",
	} {
		_, err := parseEntityDescriptor([]byte(bad))
		if err == nil {
			t.Errorf("parseEntityDescriptor(%q) returned nil error", bad)
		}
	}
}

// TestEncryptDecryptSPKeyRoundTrip mirrors the OIDC client_secret
// round-trip. Same KEK shape, distinct label.
func TestEncryptDecryptSPKeyRoundTrip(t *testing.T) {
	hubSecret := []byte("test-hub-secret-32-bytes-AAAAAAA")
	plaintext := "-----BEGIN PRIVATE KEY-----\nABC\n-----END PRIVATE KEY-----"
	enc, err := EncryptSPKey(plaintext, hubSecret)
	if err != nil {
		t.Fatalf("EncryptSPKey: %v", err)
	}
	if enc == "" {
		t.Fatal("EncryptSPKey returned empty ciphertext")
	}
	got, err := DecryptSPKey(enc, hubSecret)
	if err != nil {
		t.Fatalf("DecryptSPKey: %v", err)
	}
	if got != plaintext {
		t.Errorf("roundtrip mismatch:\n got  %q\n want %q", got, plaintext)
	}
}

// TestEncryptSPKey_DistinctFromOIDC: the same hub secret encrypted
// under the OIDC and SAML labels must produce different ciphertexts
// (and different keys). This guards against a future regression
// where someone copy-pastes the OIDC crypto and forgets to change
// the label.
func TestEncryptSPKey_DistinctFromOIDC(t *testing.T) {
	hubSecret := []byte("test-hub-secret-32-bytes-AAAAAAA")
	plaintext := "shared-plaintext"
	encSAML, err := EncryptSPKey(plaintext, hubSecret)
	if err != nil {
		t.Fatalf("EncryptSPKey: %v", err)
	}
	encOIDC, err := EncryptSecret(plaintext, hubSecret)
	if err != nil {
		t.Fatalf("EncryptSecret (OIDC): %v", err)
	}
	if encSAML == encOIDC {
		t.Fatal("SAML and OIDC encryption produced the same ciphertext — KEK labels are not distinct")
	}
}

// TestSplitCommaList + TestJoinCommaList: round-trip the
// expected_nameid wire field. Trim, dedupe, drop empties.
func TestSplitCommaList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b , ", []string{"a", "b"}},
		{"a,a,b", []string{"a", "b"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, c := range cases {
		got, err := splitCommaList(c.in)
		if err != nil {
			t.Errorf("splitCommaList(%q) errored: %v", c.in, err)
			continue
		}
		if !equalSlices(got, c.want) {
			t.Errorf("splitCommaList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestJoinCommaList_RoundTrip(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"a"}, {"a", "b"}, {"a", "b", "c"}} {
		joined := joinCommaList(in)
		got, err := splitCommaList(joined)
		if err != nil {
			t.Errorf("roundtrip for %v errored: %v", in, err)
			continue
		}
		if !equalSlices(got, in) && !(len(got) == 0 && len(in) == 0) {
			t.Errorf("roundtrip for %v lost data: joined=%q split=%v", in, joined, got)
		}
	}
}

func TestDefaultSPEntityID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://lumen.example.com", "https://lumen.example.com/api/auth/saml/metadata", false},
		{"https://lumen.example.com/", "https://lumen.example.com/api/auth/saml/metadata", false},
		{"http://localhost:8090", "http://localhost:8090/api/auth/saml/metadata", false},
		{"", "", true},
		{"not-a-url", "", true},
	}
	for _, c := range cases {
		got, err := DefaultSPEntityID(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("DefaultSPEntityID(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("DefaultSPEntityID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRSAPrivateKeyFromPEM_RejectsJunk(t *testing.T) {
	for _, bad := range []string{"", "not-pem-at-all", "-----BEGIN RSA PRIVATE KEY-----\nNOPE\n-----END RSA PRIVATE KEY-----"} {
		if _, err := parseRSAPrivateKeyFromPEM(bad); err == nil {
			t.Errorf("parseRSAPrivateKeyFromPEM(%q) returned nil error", bad)
		}
	}
}

// equalSlices is a small helper for the comma-list round-trip tests;
// we don't want to import reflect just for this.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
