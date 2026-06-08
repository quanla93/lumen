---
title: SAML SSO
description: Configure SAML2 sign-in for older enterprise + EDU IdPs (Okta classic, Azure AD enterprise, ADFS, Shibboleth). Use OIDC for new deployments when you can.
---

SAML2 unblocks the older enterprise + EDU IdP estate where OIDC is
absent or behind a higher tier: Okta classic, Microsoft Entra (Azure
AD) enterprise apps, on-prem ADFS, Shibboleth, OneLogin classic —
places where OIDC either isn't surfaced to the user or is gated.

> **Use OIDC if you can.** v0.7.0's OIDC SSO is the better default
> for new deployments — simpler setup, modern tooling, and no XML
> surface area. SAML is the bridge for the IdP estate where OIDC
> isn't an option. The two coexist: enable both if your users have
> mixed IdPs.

## Prerequisites

- A hub deployed with `LUMEN_HUB_PUBLIC_URL` set to the
  externally-reachable URL the IdP's browser will land on (e.g.
  `https://lumen.example.com`). Without this the SAML flow 503s
  because the SP can't mint a valid AuthnRequest.
- An admin user row with the same username as the NameID your IdP
  releases. Single-admin today; multi-user (Sprint 9) will extend
  SAML to honour group attributes for role assignment.
- The IdP's metadata XML (paste) or a URL that serves it (we
  refetch periodically when set).

## Recipe — generic

1. **Settings → SAML** in the hub UI.
2. Flip **Enable SAML sign-in** on.
3. Pick **IdP metadata source**:
   - **Paste XML** for the metadata document the IdP exposes.
   - **Fetch URL** for a URL the IdP serves the metadata at. The
     hub will refetch every hour so a cert rollover doesn't break
     login.
4. Paste the XML (or the URL). The hub auto-generates a 2048-bit RSA
   SP keypair + self-signed cert on first save. The cert is what
   you hand the IdP in the next step.
5. **Expected NameID** is the exact `email` (or UPN) your IdP
   releases for the admin user. Comma-separate for multiple
   bindings (e.g. `[email protected], [email protected]`). Case-insensitive.
6. Click **Test metadata** — the discovered SSO URL + IdP entity ID
   appear inline so you can confirm the input is sane before saving.
7. **Save**.
8. Copy the **SP metadata URL** (top of the form) and add it as a
   new SAML application in your IdP. Or, if the IdP accepts a
   metadata URL, paste it directly — they read the EntityID,
   AssertionConsumerService URL, and signing cert from there.

## Recipe — Okta classic

1. Okta admin → **Applications → Create App Integration → SAML 2.0**.
2. **Single sign on URL**: `https://<your-hub>/api/auth/saml/acs`
3. **Audience URI (SP Entity ID)**: paste the SP metadata URL
   (defaults to that URL itself).
4. **Name ID format**: `EmailAddress`.
5. **Application username**: `Email`.
6. **Attribute statements**: leave empty (Lumen only reads the
   NameID today).
7. **Feedback**: skip.
8. **Assignments**: assign the user(s) who should be able to log in
   to Lumen.
9. **Sign on → View SAML setup instructions** — copy the
   **Identity Provider metadata** XML. Paste into the hub's
   **IdP metadata XML** field.
10. **Expected NameID** in the hub = the email Okta releases (the
    one in step 5).
11. Save in both.

## Recipe — Azure AD enterprise app

1. Azure AD admin → **Enterprise applications → New application →
   Create your own application → Integrate any other application
   you don't find in the gallery (Non-gallery)**.
2. **Single sign-on → SAML**.
3. **Basic SAML Configuration**:
   - **Identifier (Entity ID)**: paste the SP metadata URL.
   - **Reply URL (Assertion Consumer Service URL)**:
     `https://<your-hub>/api/auth/saml/acs`.
   - **Sign on URL**: leave empty.
4. **Attributes & Claims**: edit the **nameid** claim to source
   attribute `user.mail`.
5. **SAML Certificates** → **Federation Metadata XML** → download.
   Paste into the hub's **IdP metadata XML** field.
6. **Expected NameID** in the hub = the user's `user.mail` value.
7. Save in both.

## Recipe — ADFS

1. ADFS management → **Relying Party Trusts → Add Relying Party
   Trust Wizard**.
2. **Data source**: "Import data about the relying party from a
   file" — but Lumen has no metadata file for you; skip this step
   and add the relying party manually in the next screens.
3. **Display name**: `Lumen`.
4. **Configure URL**: paste the SP metadata URL.
5. **Configure identifier**: paste the SP metadata URL (same value).
6. **Configure authentication**: leave at default (no MFA).
7. **Choose issuance authorization rules**: "Permit everyone".
8. **Finish** — close the wizard.
9. Right-click the new trust → **Properties → Endpoints**:
   - **Assertion Consumer Service**: set URL to
     `https://<your-hub>/api/auth/saml/acs`, binding = POST.
10. Right-click → **Properties → Signature**: ADFS will sign
    responses with its own cert. The hub reads that cert from the
    metadata, so make sure the next step publishes it.
11. **ADFS → Service → Endpoints → Metadata**:
    `https://<adfs>/FederationMetadata/2007-06/FederationMetadata.xml`
    — paste this URL into the hub's **Fetch URL** field (no need to
    download + paste XML; the hub refetches every hour).
12. **Expected NameID** in the hub = the LDAP attribute you
    configured ADFS to release as the NameID (commonly
    `E-Mail-Addresses`).

## Recipe — Shibboleth

1. Shibboleth IdP → `metadata/idp-metadata.xml` (the IdP's own
   metadata file). Copy the contents and paste into the hub.
2. Configure the SP via the Shibboleth relying-party config; the
   SP entity ID is the hub's metadata URL.
3. **Expected NameID** in the hub = the `urn:oid:0.9.2342.19200300.100.1.3`
   (mail) value, or the eduPersonPrincipalName, depending on what
   your IdP releases.

## Recipe — `samltest.id` (trial)

[samltest.id](https://samltest.id) is a public test IdP. Useful for
a pre-merge smoke test against a real SAML flow without setting up
your own IdP.

1. Visit samltest.id → **Connect**.
2. Copy the metadata URL it gives you.
3. Paste into the hub's **Fetch URL** field.
4. **Expected NameID** in the hub = whatever samltest.id's default
   `nameid-format=emailAddress` value is. The IdP's "Get attributes
   for user" debug page will show you the exact string.

## Troubleshooting

### "Audience mismatch"

The IdP's `Audience` element in the SAMLResponse must equal the hub's
SP entity ID. If you didn't set one explicitly, the hub defaults to
the SP metadata URL — make sure the IdP has the **exact same string**
configured as the relying-party identifier.

Fix: either (a) set the SP entity ID in the hub to match what the
IdP has, or (b) reconfigure the IdP's relying-party identifier to
the metadata URL.

### "Signature failed"

The IdP's signing cert (in its metadata) doesn't match the cert the
IdP actually signed the response with. Common after a cert rollover
where the operator forgot to update the IdP's published metadata.

Fix: re-download the IdP's current metadata XML and re-paste (or
hit the metadata URL again — the hub will refetch on the next
heartbeat).

### "NotOnOrAfter in the past" / "NotBefore in the future"

Clock skew between the hub and the IdP. Default tolerance is 60 s
(adjustable via **Clock skew (seconds)** in the SAML tab).

Fix: NTP on both hosts. If you can't, raise the clock skew setting
to cover the worst observed drift — but understand this also widens
the window for replay attacks.

### "NameID not in expected_nameid list"

The IdP released a NameID value (an email, a UPN, a transient
identifier) that isn't in your hub's comma-separated list. Common
when:

- The IdP releases `urn:oid:0.9.2342.19200300.100.1.3` (mail) but
  the operator typed the user's UPN in expected_nameid.
- The IdP sends the email in mixed case and the operator's
  expected_nameid list has it in lowercase. (Lumen is
  case-insensitive on NameID match, so this is a less common case.)

Fix: copy the exact NameID value from the IdP's debug view
(Okta: **SAML Tracer** + browser dev tools; Azure AD: **Sign-in
logs**; ADFS: Event Viewer). Paste it into the hub's
**Expected NameID** field, save, try again.

### "Replay attack" / "missing relay state cookie"

The user's browser blocked the `lumen_saml_relay` cookie. Causes:
Safari ITP, aggressive privacy extensions, or the hub running on
HTTP (the cookie is `Secure` over HTTPS). Hub must be reachable
over HTTPS for production SAML.

### Operator changed `LUMEN_HUB_SECRET` and now SAML is broken

Same constraint as OIDC client_secret + Web Push VAPID + Backup S3
secret_key — rotating the hub secret locks the four at-rest values
out. Re-issuing the SP keypair via the Settings tab is the
recovery: flip enabled off → save → flip back on → save. The hub
generates a fresh keypair, encrypts it with the new hub secret, and
the next SAML login round-trips.

## What the feature does NOT do

- **Encrypted assertions.** Many IdPs allow toggling; v1 requires
  "signed, unencrypted". v0.7.2 ships the simplest interoperable
  subset.
- **Single Log Out (SLO).** Lumen logout clears the local session
  cookie only; that's enough for homelab. v2 when a user asks.
- **IdP role.** Lumen is not an IdP for other apps — out of
  product scope.
- **Attribute mapping to roles.** Lumen binds only to the existing
  single admin row. Multi-user + TOTP (Sprint 8) extends this.
- **SP-signed AuthnRequest.** Auto-generated key+cert exist; we
  don't sign by default. If your IdP demands it, set
  `saml.sp_sign_request=true` in a follow-up.
- **Just-in-time provisioning.** v1 binds to the existing admin
  row only.
- **IdP-initiated SSO.** v1 is SP-initiated only.
