# RFC 0002 — SAML2 SSO (single-admin)

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 2
- **Effort**: 5 days
- **Author**: 2026-06-04 planning session

## Motivation

OIDC (RFC 0007's spiritual predecessor — shipped v0.7.0) covers the modern IdP world. SAML2 unblocks the older enterprise + EDU IdP estate: Okta classic, Microsoft Entra (Azure AD) enterprise apps, on-prem ADFS, Shibboleth, OneLogin classic — places where OIDC either isn't surfaced to the user or is gated behind a higher tier. A homelab user with a corporate SAML IdP at work can connect Lumen the same way they connect Jira or Confluence.

SAML is XML, complicated, and easy to misimplement. v1 sticks to **the simplest interoperable subset**: SP-initiated auth-code flow, signed assertions, no encryption, no SLO. Anything more lands when a user actually asks.

## Scope

**In scope**
- SAML SP role using `github.com/crewjam/saml`.
- SP-initiated login flow (browser → hub → IdP → hub callback).
- Signed assertion validation (NotOnOrAfter, Audience, Subject NameID).
- Single-admin gate: only the configured `expected_nameid` value passes; everyone else rejected at ACS regardless of IdP outcome.
- IdP metadata XML configured via paste / URL fetch (admin's choice).
- SP private key + cert auto-generated on first save, encrypted at rest with the hub secret (same KEK pattern as OIDC client_secret).
- SP metadata XML exposed at `/api/auth/saml/metadata` so the IdP can ingest it.
- Settings → SAML tab + Login form button (parallel to OIDC).

**Out of scope**
- Encrypted assertions. Many IdPs allow toggling; v1 requires "signed, unencrypted".
- Single Log Out (SLO). Lumen logout clears the local session cookie only; that's enough for homelab.
- IdP role (Lumen acting as an IdP for other apps) — not in product scope.
- Attribute mapping to roles / multi-user (lands with Sprint 9's multi-user + RBAC).
- SP-signed AuthnRequest by default. Generate the SP key+cert pair so we *can* sign if the IdP demands it; the request is unsigned otherwise.
- Just-in-time provisioning. v1 binds only to the existing single admin row (same model as OIDC).

## Design

### Library

`github.com/crewjam/saml` (~1.5 MB binary impact, MIT, well-maintained). Specifically `samlsp` for the SP middleware + `saml` for low-level assertion parsing.

### Migration

`internal/hub/storage/migrations/0021_saml_settings.sql` — seed-only; uses the existing `settings` k/v table.

Keys:

| Key | Description |
|---|---|
| `saml.enabled` | bool |
| `saml.idp_metadata_xml` | Raw IdP metadata XML the admin pasted or the test endpoint fetched. |
| `saml.idp_metadata_url` | Optional — if set, hub refetches periodically (default 1 h) to pick up cert rollovers. |
| `saml.sp_entity_id` | Defaults to the hub's public URL (`https://<host>/api/auth/saml/metadata`) when empty. |
| `saml.expected_nameid` | The exact NameID value (typically email) allowed to log in. |
| `saml.sp_private_key_enc` | SP signing key, AES-GCM-encrypted with the hub secret (same KEK scheme as OIDC client_secret + Web Push VAPID private — distinct labels). |
| `saml.sp_cert` | SP public cert, plaintext. |
| `saml.allowed_clock_skew_seconds` | Default `60`. Tolerance on `NotOnOrAfter` / `NotBefore`. |

### Endpoints

| Verb + path | Auth | Purpose |
|---|---|---|
| `GET /api/auth/saml/login` | none | Builds AuthnRequest, sets RelayState cookie, 302s to the IdP's SSO URL. |
| `POST /api/auth/saml/acs` | none | Assertion Consumer Service. Parses + validates the SAMLResponse, checks NameID, mints `lumen_session`, redirects to `/`. |
| `GET /api/auth/saml/metadata` | none | Returns SP metadata XML for IdP ingest. |
| `GET /api/settings/saml` | session | Returns config; `sp_private_key_enc` replaced by `has_sp_keypair: bool`. |
| `PUT /api/settings/saml` | session | Saves config. Auto-generates SP keypair on first enable if absent. |
| `POST /api/settings/saml/test-metadata` | session | Parses pasted XML or fetches the URL; returns `{ok, error?}` and the discovered SSO URL + IdP entity ID for confirmation. |

### Package layout

`internal/hub/auth/saml.go`
- `SAMLConfig` struct + `LoadSAMLConfig` / `SaveSAMLConfig`.
- `SAMLFlow` cached SP middleware keyed by `(idp_metadata_xml, sp_cert)` with hot-reload on settings change.
- `LoginRedirect(ctx, w, r) (string, error)`.
- `HandleACS(ctx, w, r) (nameID string, err error)`.
- `Metadata() ([]byte, error)`.
- `TestMetadata(ctx, xmlOrURL) (sso_url, idp_entity, error)`.

`internal/hub/auth/saml_handlers.go`
- HTTP handlers wrapping the flow + settings.

### Flow

```
browser ──GET /api/auth/saml/login──────────────▶ hub
                                                  │ build AuthnRequest with
                                                  │ unique ID + IssueInstant
                                                  │ RelayState cookie carries
                                                  │ a nonce (5-min TTL, AES-
                                                  │ GCM encrypted via hub
                                                  │ secret — same as OIDC
                                                  │ state cookie pattern)
                                                  │
                                                  ◀── 302 to IdP SSO URL
browser ──GET <idp sso>──────────────────────────▶ IdP
                                                  │ user authenticates
                                                  ◀── 302 POST to /api/auth/saml/acs
browser ──POST /api/auth/saml/acs────────────────▶ hub
                                                  │ - validate XML signature
                                                  │ - check Audience matches
                                                  │   SP entity ID
                                                  │ - check NotOnOrAfter
                                                  │ - check NotBefore (-skew)
                                                  │ - extract NameID
                                                  │ - reject if NameID ≠ expected
                                                  │ - mint lumen_session
                                                  │
                                                  ◀── 302 to /
```

On any failure the user is bounced back to `/login?sso_error=<reason>` (same UX as OIDC).

### SP key + cert auto-generation

First time `PUT /api/settings/saml` is called with `enabled=true` and no `saml.sp_cert` row:
1. Generate 2048-bit RSA key.
2. Self-sign a cert with CN = SP entity ID, validity 10 years.
3. Encrypt the private key with the hub secret (label `lumen/saml/v1`).
4. Store both.

The cert is included verbatim in `/api/auth/saml/metadata` so the IdP knows what to trust for signed responses.

### Frontend

`web/src/components/Settings.tsx` adds a "SAML" tab next to "SSO":
- Enable toggle.
- IdP metadata source select (Paste XML / Fetch URL).
- Paste field OR URL field (conditional).
- Expected NameID input with hint "the exact `email` claim your IdP releases".
- "Test metadata" button → POST `/api/settings/saml/test-metadata`. On success show the discovered SSO URL + IdP entity ID inline.
- "View SP metadata" link → `/api/auth/saml/metadata` (downloads XML).
- Save button.

`web/src/components/LoginForm.tsx`: `setup-status` extended to return `saml_enabled` alongside `oidc_enabled`. Login form renders both buttons when both enabled.

`web/src/i18n/messages.ts`: 4-5 new keys (`auth.signInWithSAML`, `auth.samlError`, `settings.tabs.saml`, plus inline labels in the SAML tab — same precedent as SSO tab: admin-only, inline English acceptable for v1).

### Testing

#### Unit (in `internal/hub/auth/saml_test.go`)
- Parse Okta-flavoured IdP metadata fixture: extract SSO URL + signing cert correctly.
- Parse Azure AD enterprise IdP metadata fixture.
- Parse ADFS-flavoured IdP metadata fixture.
- Validate a known-good signed SAMLResponse: succeeds.
- Validate the same response with an altered byte: signature check fails.
- Validate response with `NotOnOrAfter` in the past (-1h): rejected.
- Validate response with `NotOnOrAfter` 30s in the past (within default 60s skew): accepted.
- Validate response with NameID ≠ expected: rejected with clear error.

#### Integration
- Public test IdP `samltest.id` — pre-merge manual check that login round-trips.

#### Smoke
- Real Okta sandbox account (one-off pre-merge).
- Real Azure AD enterprise app (one-off pre-merge if available).

## Risks

| Risk | Mitigation |
|---|---|
| crewjam/saml API quirks around assertion encryption + transient errors | Pin a known-good version; gate encrypted assertions as explicit out-of-scope so we don't trip the path. |
| XML canonicalization edge cases on signature validation | Use crewjam's defaults; add fixture-based regression tests for the 3 IdPs we ship recipes for. |
| Some IdPs require SP-signed AuthnRequest | Auto-generate SP key+cert; sign requests when the IdP metadata declares `WantAuthnRequestsSigned=true`. |
| Time skew between hub + IdP | `saml.allowed_clock_skew_seconds` setting (default 60). Document the symptom + the knob in troubleshooting. |
| Replay attack via captured SAMLResponse | crewjam validates `InResponseTo` against our generated request ID + RelayState cookie. Document that the cookie is mandatory (no SameSite=Strict pitfall). |
| `LUMEN_HUB_SECRET` rotation breaks `sp_private_key_enc` | Same constraint as OIDC; documented + cross-linked. Operator can re-issue the SP keypair from the Settings tab to recover. |
| Operator pastes IdP metadata for a *different* app and the audience check silently passes | The audience check uses our SP entity ID, which is hub-public-URL-derived — won't match a foreign app. Test covers this. |

## Docs deliverables

- `docs/configure/saml.md`:
  - Why SAML when you already have OIDC.
  - Compatibility matrix (IdPs we've smoke-tested).
  - Step-by-step recipes: Okta classic, Azure AD enterprise, ADFS, Shibboleth, `samltest.id` (for trial).
  - "Use OIDC if you can" callout at the top — better default for new deployments.
  - Troubleshooting (clock skew, signature failed, audience mismatch, NameID format mismatch).
- `CHANGELOG.md` on ship.
- `ACTION_PLAN.md` checkbox flip + `[x] SAML2 evaluation` graduates to `[x] SAML2 shipped`.

## Open questions

1. Should we ship a "test login" button that actually round-trips against the configured IdP (not just discovery)? Proposed: yes — opens a popup, validates the full flow, closes on success. Adds ~0.5 d but worth the operator confidence.
2. SP entity ID auto-default vs require explicit? Proposed: auto-default to `https://<hub-public-url>/api/auth/saml/metadata` so operators don't trip "Audience mismatch" on first try.
3. Periodic IdP metadata refresh — Proposed: 1 h default when `saml.idp_metadata_url` is set, configurable, off by default.
4. Should `expected_nameid` accept multiple values for the single admin? (e.g., personal + work email both bind to the same account.) Proposed: comma-separated, intersect any in the list.

## Related

- `ACTION_PLAN.md` § "Phase 8 — Sprint queue".
- `internal/hub/auth/oidc.go` is the reference for the auth flow + cookie + secret-at-rest patterns.
- Multi-user (Sprint 9) will extend SAML to honour group attributes for role assignment.
