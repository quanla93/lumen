// hosts.ts — pure helpers for the OnboardingWizard (Sprint 5 / RFC 0005).
//
// Extracted out of the component so the step-visibility matrix is
// unit-testable without rendering React. The component is a thin
// shell over `computeCurrentStep` + `shouldShowWizard`.

import type { Host } from "@/lib/api";

// Step numbers. Steps 1-4 map 1:1 to RFC 0005 §"Steps". null = the
// wizard should not render (operator has either completed first-run
// or explicitly dismissed).
export type OnboardingStep = 1 | 2 | 3 | 4 | null;

// hasAnyMetrics returns true when at least one host has reported
// in (i.e. last_seen_at is non-null AND non-empty). Pure so tests
// can pin the boundary without spinning up the hub.
export function hasAnyMetrics(hosts: Host[] | undefined | null): boolean {
  if (!hosts || hosts.length === 0) return false;
  return hosts.some(
    (h) => typeof h.last_seen_at === "string" && h.last_seen_at.length > 0,
  );
}

// computeCurrentStep picks which step of the wizard to show given
// the current host + dismissed-state. The transition matrix:
//
//   no admin         → 1 (welcome + create admin)
//   admin, no hosts  → 2 (add first host)
//   admin, hosts,    no metrics → 3 (install agent)
//   admin, hosts,    at least one metric → 4 (wait — but the
//     wizard auto-closes on first metric, so this case is
//     reachable only briefly during the 3s poll cadence; we still
//     show step 4 to keep the operator oriented)
//   dismissed_at set → null (no wizard, replay required)
//
// We pass `hasAdmin` as a parameter rather than reading from auth
// state because App.tsx already gates on `view.kind === "app"` (which
// is only reachable when admin exists). Calling computeCurrentStep
// from a non-app view is a programmer error — but the function
// tolerates hasAdmin=false by returning step 1 so unit tests don't
// need to set up the whole App.
export function computeCurrentStep(
  hosts: Host[] | undefined | null,
  dismissedAt: string | null | undefined,
  hasAdmin: boolean,
): OnboardingStep {
  if (dismissedAt) return null;
  if (!hasAdmin) return 1;
  if (!hosts || hosts.length === 0) return 2;
  if (!hasAnyMetrics(hosts)) return 3;
  return 4;
}

// shouldShowWizard is the App.tsx gate. Returns true when the wizard
// should mount, false when the operator should see the dashboard
// directly. Equivalent to `computeCurrentStep(...) !== null` but
// spelled out so callers don't have to compare against null.
export function shouldShowWizard(
  hosts: Host[] | undefined | null,
  dismissedAt: string | null | undefined,
  hasAdmin: boolean,
): boolean {
  return computeCurrentStep(hosts, dismissedAt, hasAdmin) !== null;
}
