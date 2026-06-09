---
title: "Passkeys (WebAuthn)"
description: "Hardware-backed passwordless login for the Lumen admin account."
---

# Passkeys (WebAuthn)

Sprint 6 / RFC 0006. Passkeys are the modern replacement for
password-only login — phishing-resistant, hardware-backed on most
devices, no shared secret to leak. Lumen's single-admin account is
the ideal place to require one.

## What works on what

| Platform | Browser | Passkey type | Notes |
|---|---|---|---|
| macOS 13+ | Safari 16+ | iCloud Keychain | Synced across the operator's Apple devices |
| macOS | Chrome / Firefox | Platform authenticator (Touch ID) | Per-device, not synced |
| Windows 10/11 | Chrome / Edge | Windows Hello | TPM-backed; per-device |
| Windows | Firefox | USB security key only | No platform authenticator in FF yet |
| iOS 16+ | Safari | iCloud Keychain | Same Apple ID → cross-device |
| iOS | Chrome | Per-device | No iCloud sync in Chrome on iOS yet |
| Android 9+ | Chrome | Google Password Manager | Synced to operator's Google account |
| Android | Firefox | USB security key only | No platform authenticator |
| Linux | Chrome | USB security key (FIDO2) | `luks` + YubiKey 5 is the typical setup |

USB security keys (YubiKey 5 / FIDO2 / SoloKey v2) work **everywhere**
— they're the safest "if you only own one authenticator" option.

## Registering a passkey

1. Sign in to Lumen.
2. Go to **Settings → Account** (the new "Passkeys" section appears
   below the password form).
3. Click **Add passkey**.
4. Enter a label (e.g. `iPhone 15 Pro`, `YubiKey 5C — desk`,
   `ThinkPad fingerprint`).
5. Your browser / OS will prompt you to complete the WebAuthn
   ceremony (Touch ID prompt, Windows Hello popup, YubiKey tap, etc.).
6. The new passkey appears in the list with the label, the
   sign-count (how many times you've signed in with it), and the
   last-used timestamp.

Multiple passkeys per user are supported — the typical setup is
"phone passkey + YubiKey" so the operator isn't locked out when
one device is missing.

## Signing in with a passkey

On the login page, the **Sign in with passkey** button appears
below the password form when the browser reports WebAuthn
support. Clicking it triggers the browser's passkey picker. Pick
the credential → the hub verifies the assertion → sets the
`lumen_session` cookie → redirects to the dashboard.

The username field is optional. For Lumen's single-admin deployment
the username is the same one you use for password login, but the
button works without typing it first.

## Deleting a passkey

Click the **Delete** button on the passkey row, then confirm. The
hub removes the row immediately — no grace period. The next sign-in
attempt with that credential fails at the assertion step.

**Safety check (RFC §"Risks" #2)**: the hub refuses to delete the
last passkey if the user has no password set. Otherwise a single
broken / lost device would lock the operator out forever. The
`last-passkey + no-password` guard is server-side so the API
returns a 409 with a clear error message.

## HTTPS requirement

WebAuthn requires HTTPS in production. **Localhost is exempt** so
the wizard works for `pnpm dev` and a local `lumen-hub` on
`http://localhost:8090`. Behind a reverse proxy (nginx, Caddy,
Cloudflare, …), make sure TLS is terminated at the proxy and the
`Origin` header is forwarded verbatim — some proxies strip or
rewrite it, which causes the WebAuthn ceremony to fail with a
mysterious "SecurityError" on the client.

## Cloned-credential detection (RFC §"Risks" #4)

Each passkey stores a monotonic counter. The hub rejects any
assertion whose counter is `<=` the stored value. If you see a
`webauthn: sign_count did not increase` error in the hub log, an
attacker has cloned your authenticator (or your authenticator is
malfunctioning). Delete the affected passkey and re-register a
fresh one.

## Cross-device sync

Passkeys synced via iCloud Keychain / Google Password Manager work
on every device signed into the same Apple ID / Google account
that originally registered the credential. The hub sees the
same `credential_id` regardless of which device signed in. Per-device
passkeys (USB security keys, Linux platform authenticators) are
bound to one device.

Lumen does **not** require cross-device sync. If you only own one
authenticator, registering a single USB security key is the safest
setup.

## Troubleshooting

- **"This browser doesn't support passkeys"** on the login page
  → your browser is too old. Update to a current release. WebAuthn
  is supported in all current Chromium / Firefox / Safari builds.
- **`SecurityError: ... 'publickey' ...` in the browser console**
  → your reverse proxy is stripping the `Origin` header. Fix the
  proxy config and retry. (For Caddy, the `forward_proxy` directive
  does this by default.)
- **The browser shows the passkey prompt but the hub returns 401**
  → the passkey's `sign_count` is desynchronized (rare; usually
  means a clock-tampering issue with the authenticator). Delete
  and re-register.
- **`webauthn: invalid config (RPID/RPName/RPOrigin required)`** in
  the hub log → `LUMEN_HUB_PUBLIC_URL` is unset or malformed. Set
  it to the externally-reachable URL (e.g. `https://lumen.example.com`)
  and restart the hub.
- **The "Add passkey" button is missing** in Settings → Account
  → the browser does not support WebAuthn at all. Use a current
  Chrome / Edge / Safari / Firefox build.

## What's NOT in scope (RFC §"Out")

- **Passkey-only mode** (disabling password after the first
  passkey is registered) — proposed as a per-user toggle but
  deferred to a future sprint. The current model is "password
  always works; passkeys are an additional factor."
- **Cross-device sync attestation** — operators can't prove
  "this credential is synced" to the hub. Privacy concern, default
  `Attestation = none`.
- **Audit trail for passkey events** — register / revoke / login
  events are appended to the hub log but not surfaced in a
  user-facing Activity tab. Follow-up sprint.
