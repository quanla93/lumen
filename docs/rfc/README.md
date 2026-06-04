# Request for Comments (RFCs)

This directory holds **feature designs** for Lumen — *what* the next sprint ships and *how* it fits the existing code. ADRs (under `../adr/`) cover architectural choices; RFCs cover concrete feature plans.

## How to read an RFC

Each RFC is one feature, one sprint. Sections:

- **Status** — Draft | Accepted | Shipped | Superseded by RFC-NNNN
- **Sprint** — Phase 8 sprint number (see `ACTION_PLAN.md`)
- **Effort** — calendar days estimate
- **Motivation** — why ship this now
- **Scope** — what's in, what's out (the "out" matters more)
- **Design** — schema changes, new endpoints, UI surfaces, library choices
- **Risks** — known unknowns + mitigations
- **Testing** — unit tests, integration tests, manual smoke
- **Docs deliverables** — files to land alongside the code
- **Open questions** — explicit "decide before coding" items

RFCs may be amended in-place during their sprint. After the sprint ships, status flips to **Shipped** and the RFC becomes historical reference.

## Index

| # | Title | Sprint | Status |
|---|---|---|---|
| [0001](0001-backup-restore.md) | Backup + restore (local / S3) | 1 | Draft |
| [0002](0002-saml-sso.md) | SAML2 SSO (single-admin) | 2 | Draft |
| [0003](0003-beszel-bundle-1.md) | GPU monitoring + process list + maintenance windows | 3 | Draft |
| [0004](0004-notification-quality.md) | Digest + per-host share + Slack-native + multi-recipient email | 4 | Draft |
| [0005](0005-onboarding.md) | First-run guided onboarding | 5 | Draft |
| [0006](0006-webauthn.md) | WebAuthn / passkey login | 6 | Draft |
| [0007](0007-i18n-polish.md) | i18n polish + translation docs + parity CI | 7 | Draft |
| [0008](0008-multi-user-totp.md) | Multi-user (admin + viewer) + TOTP 2FA | 8 | Draft |
| [0009](0009-grafana-integration.md) | External API export + Grafana spike | 9 | Draft |

Cold tier tracked separately as **ADR 0004 — DuckDB feasibility** when demand surfaces. Windows agent dropped from the queue per operator decision 2026-06-04 (homelab fleet stays Linux + macOS).

## When to write an RFC vs an ADR

| | RFC | ADR |
|---|---|---|
| What | A feature design | An architectural choice |
| When | Sprint-start, before coding | At a turning point (storage engine, protocol, language) |
| Reversibility | Reversible — re-RFC if the feature evolves | Hard to reverse — re-ADR supersedes |
| Audience | The current operator + future contributors | Future contributors who'd ask "why like this?" |
| Lifecycle | Draft → Shipped → reference | Always alive |
