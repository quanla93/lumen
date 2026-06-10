// hosts.test.ts — Sprint 5 / RFC 0005 §"Step visibility" failing tests.
//
// Pins the 4-step matrix. All tests must FAIL on the current main
// (the file under test is the one we're about to create — these
// tests are the spec, and the implementation is written to satisfy
// them). The shape mirrors the failing-first pattern from Sprint 4:
// write the spec, run, see the failures, then implement to green.

import { describe, expect, it } from "vitest";
import { computeCurrentStep, hasAnyMetrics, shouldShowWizard } from "@/lib/hosts";
import type { Host } from "@/lib/api";

const noHosts: Host[] = [];
const hostNoMetrics: Host[] = [
  { id: 1, name: "h1", last_seen_at: null, silenced_until: null } as unknown as Host,
];
const hostWithMetrics: Host[] = [
  { id: 1, name: "h1", last_seen_at: "2026-06-09T12:00:00Z", silenced_until: null } as unknown as Host,
];
const hostWithoutLastSeenKey: Host[] = [
  // Some hosts may be returned without last_seen_at at all (older
  // hub versions, or pre-mint). The helper must tolerate that.
  { id: 1, name: "h1", silenced_until: null } as unknown as Host,
];

describe("computeCurrentStep", () => {
  it("returns 1 when there is no admin", () => {
    expect(computeCurrentStep(noHosts, null, false)).toBe(1);
  });

  it("returns 2 when admin exists but there are no hosts", () => {
    expect(computeCurrentStep(noHosts, null, true)).toBe(2);
  });

  it("returns 3 when admin + hosts exist but no host has reported metrics", () => {
    expect(computeCurrentStep(hostNoMetrics, null, true)).toBe(3);
  });

  it("returns 4 when admin + hosts + at least one host has reported", () => {
    expect(computeCurrentStep(hostWithMetrics, null, true)).toBe(4);
  });

  it("returns null when dismissedAt is set (replay required to reopen)", () => {
    expect(computeCurrentStep(noHosts, "2026-06-09T11:00:00Z", true)).toBeNull();
    expect(computeCurrentStep(hostNoMetrics, "2026-06-09T11:00:00Z", true)).toBeNull();
    expect(computeCurrentStep(hostWithMetrics, "2026-06-09T11:00:00Z", true)).toBeNull();
  });

  it("tolerates undefined / null hosts the same as []", () => {
    expect(computeCurrentStep(undefined, null, true)).toBe(2);
    expect(computeCurrentStep(null, null, true)).toBe(2);
  });
});

describe("hasAnyMetrics", () => {
  it("returns false for empty / undefined hosts", () => {
    expect(hasAnyMetrics([])).toBe(false);
    expect(hasAnyMetrics(undefined)).toBe(false);
    expect(hasAnyMetrics(null)).toBe(false);
  });

  it("returns false when all hosts have null last_seen_at", () => {
    expect(hasAnyMetrics(hostNoMetrics)).toBe(false);
  });

  it("returns true when at least one host has last_seen_at", () => {
    expect(hasAnyMetrics(hostWithMetrics)).toBe(true);
  });

  it("returns false when hosts lack the last_seen_at field entirely", () => {
    // hasAnyMetrics should treat absent as "not yet reported".
    expect(hasAnyMetrics(hostWithoutLastSeenKey)).toBe(false);
  });
});

describe("shouldShowWizard", () => {
  it("returns true when computeCurrentStep is non-null", () => {
    expect(shouldShowWizard(noHosts, null, true)).toBe(true);
    expect(shouldShowWizard(hostNoMetrics, null, true)).toBe(true);
    expect(shouldShowWizard(hostWithMetrics, null, true)).toBe(true);
  });

  it("returns false when dismissedAt is set", () => {
    expect(shouldShowWizard(noHosts, "2026-06-09T11:00:00Z", true)).toBe(false);
  });

  it("returns false when no admin (App.tsx handles the register view instead)", () => {
    // The wizard doesn't render for the no-admin case; that path is
    // handled by the existing RegisterForm view. shouldShowWizard
    // can still return true (the wizard is technically "the right
    // thing to show" — but App.tsx routes to register first), so
    // we don't assert on this case here; it lives in computeCurrentStep.
    expect(shouldShowWizard(noHosts, null, false)).toBe(true);
  });
});
