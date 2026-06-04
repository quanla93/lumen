# RFC 0007 — i18n polish + translation docs + parity CI

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 7
- **Effort**: 3 days

## Motivation

Recent admin-only surfaces (Settings → SSO, Settings → Status page, Settings → SAML, the WebPushPanel in Alerts, status-page-related strings on the Public status page) shipped with inline English labels. Reasoning: those surfaces are admin-only, rare, and English is the lingua franca for technical setup. That tradeoff was fine at the rate of one tab per sprint; we now have five such surfaces, and a VI-speaking admin who configures any of them hits an inconsistent UI.

Sprint 7 audits all inline strings, promotes the worth-promoting ones to `messages.ts`, adds a contribution guide, and enforces EN/VI key parity in CI so future drift is mechanical to catch.

## Scope

**In**:
- Grep all `\.tsx` + `\.ts` for hardcoded English strings inside JSX or `string` literals destined for UI.
- Promote each to `messages.ts` with EN + VI translations.
- New namespace `settings.sso`, `settings.statusPage`, `settings.saml`, `alerts.webPush`, `publicStatus.*`.
- CI script: parse `messages.ts`, assert `en` and `vi` have identical key shapes; fail otherwise.
- `docs/contributing/i18n.md` — how to add a key, how to add a third locale.

**Out**: Adding new locales (no resources to maintain). Date/number pluralization library (overkill for current scope). RTL support.

## Design

### Audit script

`scripts/i18n-audit.mjs`:
- Walks `web/src/**/*.{ts,tsx}`.
- For each JSX expression child that is a string literal OR a string template with no interpolation: flag if the string contains ≥ 3 ASCII letters AND isn't already inside `t(…)`.
- Output: file:line + the string.
- Used pre-sprint to enumerate work; CI uses a stricter variant (allow-list of intentional inline strings — code identifiers, units like `kB`).

### Parity CI

`scripts/i18n-parity.mjs`:
- `import { en, vi } from "@/i18n/messages"`.
- Deep-walk both; compare key sets at every level.
- Print diff; exit 1 if any divergence.
- Wired into `.github/workflows/ci.yml` as a step in the existing web job.

### Promotion plan

Per surface:
- Replace inline labels with `t("settings.sso.title")` etc.
- Add EN + VI entries to `messages.ts`.
- Localize success/error toasts, button labels, placeholder hints, help text.
- Keep code identifiers (URLs, field names like `client_id`) as-is.

### Contribution guide

`docs/contributing/i18n.md`:
- File layout (`web/src/i18n/messages.ts`).
- How to add a key (EN first, VI second, CI enforces both).
- How to add a third locale (subclass the `WidenStrings` type, add a new top-level object).
- Style notes (avoid sentence-case differences in EN headings vs button labels; VI uses ASCII letters mostly so no special font work).

### Translation owner pattern

VI is owned by the project author. Future locales should be owned by a maintainer commit-reviewing PRs that touch their language. Document in the guide.

## Risks

| Risk | Mitigation |
|---|---|
| Parity CI false positives on intentional EN-only keys (e.g., brand names) | Allow-list under a `__verbatim` key with a comment. |
| Audit script flags too many spurious strings | Iterative — add allow-list entries; document the workflow. |
| VI translations of technical jargon are awkward | Acceptable; the guide encourages keeping brand/protocol names verbatim (OIDC, SAML, S3, VAPID, etc.). |

## Testing

- Snapshot tests for selected pages in both locales to catch missing keys.
- CI parity script.

## Docs deliverables

- `docs/contributing/i18n.md`.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. Should the audit script be a `make` target? Proposed: `make i18n-audit` + `make i18n-check` (CI uses the latter).
2. Use ICU MessageFormat or stay with the current `{name}`-style templating? Proposed: stay — the project's templating covers ≥99% of needs and ICU adds dep weight.
