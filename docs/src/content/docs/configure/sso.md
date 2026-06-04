---
title: Single sign-on (OIDC)
description: "Bind a self-hosted IdP to Lumen so the admin signs in via OIDC instead of a local password. Single-admin scope; password login keeps working as a fallback."
sidebar:
  order: 6
---

Lumen ships with a single-admin user model. SSO via OIDC lets you sign in with your existing identity provider (Authentik, Keycloak, Google, Okta, Microsoft Entra, …) instead of the local password — useful when you already centralize your home or team identity, or when you want hardware-key / WebAuthn on top of Lumen without Lumen having to ship its own.

**The local password keeps working as a fallback** for the configured admin even when SSO is on. There's no recovery flow for the password yet, and OIDC misconfiguration shouldn't lock you out of your own monitoring.

## Single-admin scope

By design:

- Only **one** identity may sign in via OIDC: the email you paste into **Settings → SSO → Expected admin email**. Every other identity is rejected at the callback, regardless of how the IdP authenticated them.
- The OIDC session binds to Lumen's local admin user (the one you created on first run). There's no separate "SSO user" — once OIDC verifies the email, the `lumen_session` cookie is issued exactly as it would be after a password login.
- Multi-user / read-only SSO is on the [roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md) but not in this release.

## Prerequisites

- Lumen hub running at a URL reachable by your browser (e.g. `https://lumen.example.lan`).
- `LUMEN_HUB_SECRET` set to a stable 32-byte hex value in your hub compose. The OIDC client secret is encrypted at rest with a key derived from `LUMEN_HUB_SECRET`; if it's regenerated on every restart, the saved client secret becomes unreadable.
- An OIDC application registered with your IdP, returning at minimum the `email` claim (so Lumen can match it against the expected admin email).
- A confidential client (Lumen uses the standard authorization-code flow with `client_secret`).

## Callback URL

Lumen's OIDC callback is:

```text
https://<your-hub-url>/api/auth/oidc/callback
```

Register that as the only allowed redirect URI in your IdP. The hub derives the scheme + host from the inbound request and honours `X-Forwarded-Proto` / `X-Forwarded-Host`, so reverse-proxy setups (nginx, Caddy, Traefik) work without extra config — make sure the proxy forwards those headers if you terminate TLS in front.

## Configure in the UI

1. Sign into Lumen with the local admin password.
2. Open **Settings → SSO**.
3. Tick **Enable OIDC login** (you can save in either state; required fields are only enforced when enabled).
4. Paste:
   - **Issuer URL** — the OIDC issuer base; usually visible as the `iss` claim in your IdP's ID tokens. For Authentik this looks like `https://authentik.example.com/application/o/lumen/`.
   - **Client ID** — from your IdP's OAuth application.
   - **Client secret** — pasted on first save, then stored encrypted at rest. Subsequent saves with a blank field keep the existing secret.
   - **Scopes** — defaults to `openid email profile`. Add others (`groups`, `offline_access`) only if your IdP needs them; Lumen ignores them.
   - **Expected admin email** — the lowercase email of the only identity allowed to sign in via OIDC. Must match the `email` claim returned by the IdP exactly.
5. Click **Test discovery** before saving to confirm the issuer URL is reachable and exposes `/.well-known/openid-configuration`. The hub doesn't follow the discovery URL until this button is clicked or OIDC is actually used, so a typo here doesn't break the password login.
6. **Save**. The hub clears its discovery cache so the new issuer / client takes effect immediately.
7. Sign out and you'll see a **Sign in with SSO** button on the login page next to the password form.

## How the flow works

```
browser ──GET /api/auth/oidc/login───────────▶ hub
                                                │ generates state + nonce, encrypts both
                                                │ into a 5-minute lumen_oidc_state cookie
                                                │
                                                ◀── 302 to IdP authorize URL
browser ──GET <idp authorize URL>──────────────▶ IdP
                                                ◀── 302 to /api/auth/oidc/callback?code=…&state=…
browser ──GET /api/auth/oidc/callback──────────▶ hub
                                                │ - validates state matches the cookie
                                                │ - exchanges code for tokens (server→IdP)
                                                │ - verifies ID token signature via JWKS
                                                │ - checks nonce
                                                │ - checks email claim == expected_email
                                                │ - mints lumen_session cookie
                                                │
                                                ◀── 302 to /
```

On any failure the user is bounced back to `/login?sso_error=<reason>`. The reason is hint-only; the login form renders a generic "SSO sign-in failed: <reason>" line and lets the user fall back to password login.

## Provider recipes

### Authentik

1. **Applications → Applications** → *Create* → Name: `Lumen`. Save.
2. **Applications → Providers** → *Create* → Type: **OAuth2/OpenID Provider**.
   - Name: `Lumen`
   - Authorization flow: `default-provider-authorization-implicit-consent` (or your preferred one)
   - Client type: **Confidential**
   - Redirect URIs / Origins: `https://lumen.example.lan/api/auth/oidc/callback`
   - Signing key: any RS256-capable cert
   - Scopes: `openid`, `email`, `profile`
   - Save. Copy the **Client ID** and **Client Secret** that Authentik shows.
3. Back on **Applications → Applications → Lumen**, set the provider to the one you just created.
4. In Lumen's **Settings → SSO**:
   - **Issuer URL**: `https://authentik.example.com/application/o/lumen/` (the trailing slash matters; copy it from Authentik's provider page).
   - **Client ID** / **Client Secret**: paste from step 2.
   - **Expected admin email**: your Authentik account's email.

### Keycloak

1. **Realm → Clients → Create client** → Client type `OpenID Connect`, Client ID `lumen`.
2. **Capability config**: Client authentication **On**, Authorization **Off**, Standard flow **On**.
3. **Login settings → Valid redirect URIs**: `https://lumen.example.lan/api/auth/oidc/callback`.
4. **Credentials tab** → copy the **Client secret**.
5. In Lumen:
   - **Issuer URL**: `https://keycloak.example.lan/realms/<realm-name>`
   - **Client ID**: `lumen`
   - **Client Secret**: from step 4
   - **Expected admin email**: your Keycloak user's email.

### Google

1. [Google Cloud Console → APIs & Services → OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent) → configure an Internal app (or External + add yourself as a test user).
2. **Credentials → Create credentials → OAuth client ID** → Type: **Web application**.
3. **Authorized redirect URIs**: `https://lumen.example.lan/api/auth/oidc/callback`.
4. Copy the **Client ID** + **Client secret**.
5. In Lumen:
   - **Issuer URL**: `https://accounts.google.com`
   - **Client ID** / **Client Secret**: from step 4
   - **Expected admin email**: the Google account email.

## Troubleshooting

**`Test discovery` returns `404` or `connection refused`**
: The issuer URL is wrong, your IdP is unreachable from the hub, or your TLS chain isn't trusted by the hub. Verify with `curl -v <issuer>/.well-known/openid-configuration` from a shell on the hub host.

**`OIDC email "x@y.com" does not match expected "z@y.com"`**
: The IdP returned a different email than what you pasted into **Expected admin email**. Either fix the field, or check the IdP's user profile — many IdPs let you set a separate "preferred email" that's distinct from the login username.

**`provider returned no email; ensure scope 'email' is granted`**
: The IdP didn't include the `email` claim. Check your application's scopes; some Authentik / Keycloak setups need an additional scope mapper to release `email`.

**`state mismatch` or `nonce mismatch`**
: The browser dropped the short-lived state cookie (set with `SameSite=Lax`, 5-minute TTL) between the redirect and the callback. Common causes: browser privacy mode wiping cookies, a reverse proxy stripping `Set-Cookie`, or taking more than 5 minutes to complete the IdP login.

**Sign-in works once but then loops**
: Check that the password login still works (always-on fallback). If you can sign in with the password but SSO loops, the IdP is probably issuing tokens with an `email_verified=false` claim or a slightly different email than expected; the callback log line (`oidc callback failed`) names the exact reason.

## Disabling SSO

Sign in with password, open **Settings → SSO**, uncheck **Enable OIDC login**, save. The next page load drops the **Sign in with SSO** button. Saved config persists — flip the checkbox back on whenever you want it active again.

To fully clear everything, delete the `oidc.*` rows from the `settings` table:

```sql
DELETE FROM settings WHERE key LIKE 'oidc.%';
```

A fresh hub install picks up no OIDC config by default, so nothing to do on day-1.
