# RFC 0005 — First-run guided onboarding

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 5
- **Effort**: 4 days

## Motivation

Today's first-run experience: `/api/setup-status` says "no admin" → `/register` form → empty dashboard, hub URL, host token form, copy-paste compose snippet from `TokenReveal.tsx`. Each transition asks the operator to know what to do next. New users hit dead-ends:

- "Hub is up but where are hosts?" — they don't realize hosts must be created.
- "I created a host but my server doesn't show up" — they didn't run the agent.
- "I ran the agent but Settings says 0 hosts" — wrong hub URL / wrong token.

A 4-step wizard turns this into a guided sequence. The wizard is dismissable once at least one metric has arrived, and replayable from Settings.

## Scope

**In**: 4-step overlay that gates the dashboard when no admin OR no hosts. Live polling so the operator sees the "first metrics received" moment. Replay button.

**Out**: Multiple-host onboarding (only the first one is guided). Onboarding for SSO / SAML / Web Push channels. Tutorials inside the dashboard ("did you know…" overlays).

## Design

### Steps

1. **Welcome + create admin** — exact same form as the current `/register` page, but framed as "Step 1 of 4". Skipped if an admin already exists.
2. **Add first host** — host name input, "Create" mints the token. On success, the token is shown ONCE with a "Copy" button and a "Got it" confirm — same flow as the existing Settings → Hosts mint, with extra prose.
3. **Install agent** — the existing `TokenReveal` tabs (Docker Compose / Binary / Windows after Sprint 5), prefilled with the token from Step 2, with a "I'll do this somewhere else, mark me waiting" button so the operator doesn't have to actually paste the snippet in front of the wizard.
4. **Wait for metrics** — poll `/api/hosts` every 3 s. On first non-empty `last_seen_at`, render a check-mark + "Open dashboard" CTA. Provide a "Skip — show me the dashboard now" button after 60 s so a stuck operator isn't trapped.

### State management

`OnboardingWizard.tsx` is an overlay rendered when `App.tsx`'s bootstrap detects:
- No admin → step 1.
- Admin but no hosts → step 2.
- Hosts but no `last_seen_at` on any → step 4 (skip 2+3 since you already have a host but it's silent).

A new `user_prefs` key `onboarding.dismissed_at` lets a returning admin dismiss the wizard even when conditions still match (e.g., they deliberately left all agents off for a holiday).

Settings → Account adds a "Replay onboarding" button — clears `onboarding.dismissed_at`, opens the wizard at step 4 (or whichever applies).

### Polling

Same SWR-style poll used by the Dashboard component — reuse the hosts query with `staleTime: 0` while the wizard is active.

### Frontend wiring

- `web/src/components/OnboardingWizard.tsx` — new component, rendered above `<AppShell>` when active.
- `web/src/App.tsx` — decide visibility in `bootstrap()`.
- `web/src/i18n/messages.ts` — full EN+VI for the wizard (operator-facing surface).

### Backend changes

Minimal:
- `user_prefs` already supports arbitrary keys; `onboarding.dismissed_at` slots in.
- The "first metrics" condition is already exposed via `/api/hosts` (`last_seen_at`).
- No new endpoints.

## Risks

| Risk | Mitigation |
|---|---|
| Operator dismisses too aggressively and misses important steps | "Skip" buttons appear only after 60 s on each step. The button warns "Some metrics may not arrive". |
| Wizard polling burns hub CPU for inactive sessions | Stop polling when window blurred (`visibilitychange` listener). Reauth aside, polling is reads — negligible cost. |
| Operator pastes the docker compose but their host has a different network reaching the hub | Add a "Connection check" sub-step inside Step 4 that runs `fetch('/api/hosts')` from the wizard tab vs the documented hub URL the operator typed earlier; surface mismatch ("Your browser sees the hub at X; agent on the target sees no hub — likely wrong URL in agent config"). |
| Wizard overlaps existing OIDC login flow | Wizard only renders inside an authenticated session; bootstrap order is: setup-status → me → wizard. |

## Testing

- Render snapshot tests for each of the 4 steps.
- E2E with Playwright: register → create host → simulate ingest → wizard auto-closes.

## Docs deliverables

- `docs/getting-started/onboarding.md` — what the wizard does + how to skip / replay.
- Updates to `docs/getting-started/quickstart.md` to point at the wizard.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. Should the wizard auto-open at the dashboard on subsequent logins if a NEW host is added but never reports? Proposed: no — that's a different problem (alerts already cover "host stale"); avoid being naggy.
2. Should the wizard offer Docker / Binary / Windows tabs based on detected target OS? Proposed: no — operator picks; we can't sniff the target.
3. Should we ship a sample "self-monitor" toggle that auto-creates a host pointing at the hub itself? Proposed: yes, optional checkbox in Step 2 ("Also monitor the hub host"). Saves a step + makes the wizard's "wait for metrics" finish without the operator going elsewhere if they're on the hub machine.
