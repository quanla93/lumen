// OnboardingWizard.test.tsx — Sprint 5 / RFC 0005 failing tests.
//
// Pins the 4-step render matrix. The pure-function tests in
// src/lib/hosts.test.ts cover the step-visibility decision; these
// tests cover the component shell — that the right title / CTA
// renders for the right step, that the dismissed state renders
// nothing, and that step 2's submit calls hostsApi.create with the
// operator's chosen name.

import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { OnboardingWizard } from "@/components/OnboardingWizard";
import { I18nProvider } from "@/i18n/I18nProvider";
import * as apiModule from "@/lib/api";
import type { Host } from "@/lib/api";

// Stub out the i18n hook so we don't need to load the real
// messages.ts. The wizard is responsible for passing the right
// translation key; the test asserts the key path, not the
// rendered string.
vi.mock("@/i18n/useI18n", async () => {
  const actual = await vi.importActual<typeof import("@/i18n/useI18n")>("@/i18n/useI18n");
  return {
    ...actual,
    useI18n: () => ({ t: (k: string) => k, locale: "en", setLocale: () => {} }),
  };
});

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

const emptyHosts: Host[] = [];
const noMetricsHost: Host[] = [
  { id: 1, name: "h1", last_seen_at: null, silenced_until: null } as unknown as Host,
];

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("OnboardingWizard", () => {
  it("renders step 2 when admin exists but no hosts", () => {
    renderWithI18n(
      <OnboardingWizard
        hosts={emptyHosts}
        dismissedAt={null}
        onDismissed={() => {}}
      />,
    );
    // Step 2 title key
    expect(screen.getByText("onboarding.step2.title")).toBeInTheDocument();
    // Step 2 CTA
    expect(screen.getByRole("button", { name: "onboarding.step2.cta" })).toBeInTheDocument();
  });

  it("renders step 3 when admin + host exist but no metrics", () => {
    renderWithI18n(
      <OnboardingWizard
        hosts={noMetricsHost}
        dismissedAt={null}
        onDismissed={() => {}}
      />,
    );
    expect(screen.getByText("onboarding.step3.title")).toBeInTheDocument();
  });

  it("renders step 4 when admin + host + at least one metric", () => {
    const withMetrics: Host[] = [
      { id: 1, name: "h1", last_seen_at: "2026-06-09T12:00:00Z", silenced_until: null } as unknown as Host,
    ];
    renderWithI18n(
      <OnboardingWizard
        hosts={withMetrics}
        dismissedAt={null}
        onDismissed={() => {}}
      />,
    );
    expect(screen.getByText("onboarding.step4.title")).toBeInTheDocument();
  });

  it("renders nothing when dismissedAt is set (replay required)", () => {
    const { container } = renderWithI18n(
      <OnboardingWizard
        hosts={noMetricsHost}
        dismissedAt="2026-06-09T11:00:00Z"
        onDismissed={() => {}}
      />,
    );
    // No Dialog.Title or wizard body should appear.
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it("calls hostsApi.create with the typed name in step 2", async () => {
    const createSpy = vi
      .spyOn(apiModule.hostsApi, "create")
      .mockResolvedValue({ host: { id: 1, name: "nas" } as unknown as Host, token: "tok" });
    vi.spyOn(apiModule.hostsApi, "list").mockResolvedValue([]);

    renderWithI18n(
      <OnboardingWizard
        hosts={emptyHosts}
        dismissedAt={null}
        onDismissed={() => {}}
      />,
    );
    const input = screen.getByPlaceholderText("onboarding.step2.placeholder");
    fireEvent.change(input, { target: { value: "nas" } });
    fireEvent.click(screen.getByRole("button", { name: "onboarding.step2.cta" }));

    await waitFor(() => expect(createSpy).toHaveBeenCalledWith("nas"));
  });
});
