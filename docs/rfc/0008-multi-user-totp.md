# RFC 0008 — Multi-user (admin + viewer) + TOTP 2FA

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 8
- **Effort**: 6-7 days

## Motivation

Lumen has been single-admin since v0.1. Two pressure points:

- **Read-only viewer**: an operator wants to share live metrics with a teammate without giving them password change / OIDC config / host creation rights. The status page (Phase 8 Sprint 3) covers public read-only, but not "logged-in, no admin powers" — viewer can see alerts history, container details, share links, dashboard prefs, things the public page doesn't expose.
- **2FA**: TOTP is the floor for serious deployments. WebAuthn (Sprint 6) is the modern path; TOTP covers everyone whose IdP isn't on board with passkeys, including the operator's password fallback.

Sprint 8 lands both in one shape — both are user-table concerns + new middleware gates.

## Scope

### Multi-user
**In**: New `role` column on `users` (`admin` | `viewer`). `RequireRole` middleware. Settings → Users tab (list, invite via one-time link, set role, revoke). SSO/SAML/WebAuthn bind via email match — first SSO login as the operator's email lands them in the existing user row.

**Out**: True multi-tenancy (data segmentation per user). Per-host ACLs (a viewer sees ALL hosts). Custom roles beyond admin / viewer. Group sync from OIDC `groups` claim (v2; document the deferral).

### TOTP
**In**: TOTP secret per user, encrypted at rest. QR enrolment. Code prompt on login. 10 one-time recovery codes generated at enrolment, hashed at rest.

**Out**: Push-style approve flows (Duo). SMS-based fallback (insecure + Twilio bill).

## Design

### Multi-user

Migration `0024_users_roles.sql`:
```sql
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin';
-- New users default to 'admin' so the single-admin migration is a no-op.
-- The Settings → Users → Invite flow sets role explicitly per invite.

CREATE TABLE user_invites (
    token       TEXT PRIMARY KEY,
    role        TEXT NOT NULL,
    expires_at  DATETIME NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    created_by  INTEGER,
    consumed_at DATETIME,
    consumed_user_id INTEGER,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (consumed_user_id) REFERENCES users(id) ON DELETE SET NULL
);
```

Middleware:
- `RequireSession` (existing) populates `userID` in context.
- `RequireRole(role)` checks `users.role == role || role == "viewer"` (admin can do viewer things).

Endpoints:
- `GET /api/users` (admin) — list users with role + last login.
- `POST /api/users/invite` (admin) — body `{role, label, ttl_hours}` returns invite URL.
- `DELETE /api/users/{id}` (admin) — refuses if it would leave zero admins.
- `PUT /api/users/{id}/role` (admin) — same refusal.
- `POST /api/users/accept-invite/{token}` (none) — creates the user, mints session.
- All existing write endpoints gain `RequireRole("admin")`. Read endpoints stay session-only.

Frontend:
- Settings → Users tab (list, invite, revoke).
- Surface "Viewer mode" badge in the AppShell when role=viewer.
- Hide write buttons (Create host, Rotate token, Settings tabs other than Account/Display/Hub status) for viewers.

### TOTP

Library: `github.com/pquerna/otp` (already implicitly in scope; if not, add).

Migration `0025_totp.sql`:
```sql
ALTER TABLE users ADD COLUMN totp_secret_enc TEXT NOT NULL DEFAULT '';
CREATE TABLE totp_recovery_codes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    code_hash   TEXT NOT NULL,
    used_at     DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

Endpoints:
- `POST /api/me/totp/setup` — generate secret + QR data URL; returns provisioning URI + 10 recovery codes (plaintext, shown ONCE).
- `POST /api/me/totp/confirm` — body `{code}` validates + persists. Refuses if not confirmed.
- `DELETE /api/me/totp` — body `{password}` removes. Forces password reauth.
- Login endpoint extended: if user has TOTP enabled, accepts `{username, password, totp_code OR recovery_code}` — refuses if missing.

Secret encryption: AES-GCM keyed off hub secret, label `lumen/totp/v1`.

Frontend:
- Settings → Account → "Enable 2FA" → modal showing QR + manual secret + code field + "Save recovery codes" confirm. Recovery codes copy-to-clipboard + warning to print/save.
- Login form: post-password, if response says `{ "need_2fa": true }`, show the TOTP code input.
- Settings → Account shows "2FA enabled" badge + "Disable" button (asks for password).

## Risks

| Risk | Mitigation |
|---|---|
| Migration with default `role='admin'` for existing rows | Correct by construction — the single existing admin should stay admin. Document. |
| Viewer who creates a share link or hits "Replay onboarding" | Both are read-only flows in practice; allow. Document. |
| Operator loses TOTP device + recovery codes | Recovery codes ARE the recovery path; document storing them. `LUMEN_HUB_SECRET` rotation invalidates `totp_secret_enc` — operator can disable+re-enable. |
| Invite link leaks | Tokens are 32-byte random, base64url; default 24 h TTL; single-use; visible only at create time. |
| Viewer role bypasses something via the API | Audit each write endpoint; tests cover. |
| WebAuthn + TOTP both enabled = double prompt | At login, allow EITHER; document. |

## Testing

- `RequireRole` middleware: admin endpoint with viewer token → 403; viewer endpoint with admin token → OK.
- Invite flow: create, accept, role correct on the new row, second use of the same token → 410.
- Delete-last-admin guard.
- TOTP fixture-based verify (clock skew ±1 window).
- Recovery code: one-time use, second use rejected.

## Docs deliverables

- `docs/configure/users-and-roles.md`.
- `docs/configure/totp.md` — enrolment, recovery codes, what to do if you lose the phone.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. Should viewers be able to TEST a channel (no permanent state change)? Proposed: no — channel test sends real notification to a real recipient (side effect). Admin-only.
2. Should an admin be able to view (read-only) the viewer's user prefs? Proposed: no — privacy.
3. SSO/SAML user lacking a corresponding local user — provision automatically or reject? Proposed: reject in v1, document; auto-provision flows under multi-user-with-groups (v2).
