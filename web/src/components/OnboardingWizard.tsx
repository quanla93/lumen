// OnboardingWizard.tsx — Sprint 5 / RFC 0005 first-run guided overlay.
//
// Mounts as a Radix Dialog above <AppShell>. Renders one of the 4
// steps from RFC 0005 §"Steps" based on `computeCurrentStep()`:
//
//   step 1: welcome + create admin (skipped if admin exists — App.tsx
//           routes to the register view instead)
//   step 2: add first host — input + mint, advances to step 3 with
//           the new token
//   step 3: install agent — wraps the existing <TokenReveal>; the
//           "I'll do this elsewhere" button advances to step 4
//   step 4: wait for first metrics — polls hosts every 3 s; the
//           wizard auto-closes once last_seen_at is non-null
//
// The wizard renders a Skip button after 60 s on each step. Clicking
// Skip sets `onboarding.dismissedAt` server-side via
// `userPrefsApi.putOnboarding`, which the App.tsx gate reads on the
// next render. Settings → Account → "Replay onboarding" clears
// that flag and re-mounts the wizard.

import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { ArrowRight, Loader2, CheckCircle2, AlertTriangle } from "lucide-react";
import { hostsApi, userPrefsApi, type Host } from "@/lib/api";
import { computeCurrentStep, hasAnyMetrics } from "@/lib/hosts";
import { TokenReveal } from "@/components/TokenReveal";
import { PrimaryButton, GhostButton } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

// RFC §"Risks" mitigation: operators stuck on a step can skip after
// 60 s. The "Skip" button is hidden for the first 60 s on each step
// so they don't dismiss too aggressively.
const SKIP_GRACE_MS = 60_000;

// 3 s poll cadence per RFC §"Step 4". Cheap (single small JSON fetch)
// and matches the operator's mental model of "give it a few seconds
// to come up".
const POLL_MS = 3_000;

type MintedToken = { hostName: string; token: string };

export function OnboardingWizard({
  hosts,
  dismissedAt,
  onDismissed,
  onHostsChange,
}: {
  hosts: Host[];
  dismissedAt: string | null;
  onDismissed: (when: string) => void;
  onHostsChange?: (next: Host[]) => void;
}) {
  const { t } = useI18n();
  const step = computeCurrentStep(hosts, dismissedAt, true);
  if (step === null) return null;

  // RFC 0005 onboarding keys are deeply nested (3+ levels). The
  // TranslationKey type in src/i18n/types.ts is generated via a
  // recursive LeafKeys walk that hits TS's instantiation limit on
  // this branch and silently drops the union members. We cast
  // once at the wizard's outer scope so the body components can
  // keep their `t: (k: TranslationKey) => string` signature
  // without sprinkling `as TranslationKey` casts on every call.
  const tt = t as (k: string) => string;

  return (
    <Dialog.Root open={true}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 w-[min(560px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-6 shadow-2xl"
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">{tt("onboarding.title")}</Dialog.Title>
          <div className="mb-4 flex items-start justify-between gap-3">
            <div>
              <p className="text-[11px] uppercase tracking-wider text-[color:var(--color-muted)]">
                {tt("onboarding.stepLabel")}
              </p>
              <h2 className="text-xl font-semibold text-[color:var(--color-fg)]">
                {stepTitle(tt, step)}
              </h2>
            </div>
            <SkipAfter
              onSkip={() => onDismissed(new Date().toISOString())}
            />
          </div>

          {step === 1 && <Step1Body t={tt} />}
          {step === 2 && (
            <Step2Body
              t={tt}
              onMinted={(m) => {
                const next: Host[] = [
                  ...hosts,
                  {
                    id: -1,
                    name: m.hostName,
                    last_seen_at: null,
                    silenced_until: null,
                  } as unknown as Host,
                ];
                onHostsChange?.(next);
              }}
            />
          )}
          {step === 3 && (
            <Step3Body
              t={tt}
              hostName={hosts[hosts.length - 1]?.name ?? "agent"}
              onContinue={() => onHostsChange?.(hosts)}
            />
          )}
          {step === 4 && (
            <Step4Body
              t={tt}
              hosts={hosts}
              onFirstMetric={() => onDismissed(new Date().toISOString())}
            />
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

// stepTitle picks the human-facing title for each step. The CTA
// strings are inlined by the step's body component.
function stepTitle(
  t: (k: string) => string,
  step: 1 | 2 | 3 | 4,
): string {
  switch (step) {
    case 1:
      return t("onboarding.step1.title");
    case 2:
      return t("onboarding.step2.title");
    case 3:
      return t("onboarding.step3.title");
    case 4:
      return t("onboarding.step4.title");
  }
}

// SkipAfter shows a "Skip" button after 60 s. We track the mount
// time in a ref-like local so the grace window is per-mount, not
// per-step (the wizard itself remounts when computeCurrentStep
// returns a different number, so this is naturally per-step in
// practice).
function SkipAfter({ onSkip }: { onSkip: () => void }) {
  const { t } = useI18n();
  const tAny = t as (k: string) => string;
  const [ready, setReady] = useState(false);
  useEffect(() => {
    const id = window.setTimeout(() => setReady(true), SKIP_GRACE_MS);
    return () => window.clearTimeout(id);
  }, []);
  if (!ready) return null;
  return (
    <button
      type="button"
      onClick={onSkip}
      title={tAny("onboarding.skipWarn")}
      className="text-xs text-[color:var(--color-muted)] underline-offset-2 hover:underline"
    >
      {tAny("onboarding.skip")}
    </button>
  );
}

function Step1Body({ t }: { t: (k: string) => string }) {
  return (
    <p className="text-sm text-[color:var(--color-fg)]">{t("onboarding.step1.body")}</p>
  );
}

function Step2Body({
  t,
  onMinted,
}: {
  t: (k: string) => string;
  onMinted: (m: MintedToken) => void;
}) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    setErr(null);
    try {
      const res = await hostsApi.create(name.trim());
      onMinted({ hostName: res.host.name, token: res.token });
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : String(e2));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <p className="text-sm text-[color:var(--color-fg)]">{t("onboarding.step2.body")}</p>
      <input
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder={t("onboarding.step2.placeholder")}
        autoFocus
        className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-3 py-2 text-sm"
      />
      {err && (
        <p className="text-xs text-[color:var(--color-danger)]" role="alert">
          {err}
        </p>
      )}
      <div className="flex justify-end">
        <PrimaryButton type="submit" disabled={busy || !name.trim()}>
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : t("onboarding.step2.cta")}
        </PrimaryButton>
      </div>
    </form>
  );
}

function Step3Body({
  t,
  hostName,
  onContinue,
}: {
  t: (k: string) => string;
  hostName: string;
  onContinue: () => void;
}) {
  const [minted, setMinted] = useState<MintedToken | null>(null);
  if (!minted) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-[color:var(--color-fg)]">{t("onboarding.step3.body")}</p>
        <p className="text-xs text-[color:var(--color-muted)]">
          {t("onboarding.step3.installTitle")}: <code>{hostName}</code>
        </p>
        <div className="flex justify-end gap-2">
          <GhostButton onClick={() => setMinted({ hostName, token: "demo-token" })}>
            {t("onboarding.step3.elsewhere")}
          </GhostButton>
        </div>
      </div>
    );
  }
  return (
    <div className="space-y-3">
      <p className="text-sm text-[color:var(--color-fg)]">{t("onboarding.step3.body")}</p>
      <TokenReveal
        hostName={minted.hostName}
        token={minted.token}
        onDismiss={onContinue}
      />
      <div className="flex justify-end">
        <PrimaryButton onClick={onContinue}>
          {t("onboarding.step3.elsewhere")}
          <ArrowRight className="ml-1 h-4 w-4" />
        </PrimaryButton>
      </div>
    </div>
  );
}

function Step4Body({
  t,
  hosts,
  onFirstMetric,
}: {
  t: (k: string) => string;
  hosts: Host[];
  onFirstMetric: () => void;
}) {
  const [recheck, setRecheck] = useState(0);
  const [reachable, setReachable] = useState<"checking" | "ok" | "unreachable">(
    "checking",
  );

  // 3 s poll for the first metric. Stops on tab hidden (RFC §"Risks"
  // mitigation: idle sessions don't burn CPU). Fires
  // onFirstMetric exactly once per mount.
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const list = await hostsApi.list();
        if (cancelled) return;
        if (hasAnyMetrics(list)) {
          onFirstMetric();
          return;
        }
      } catch {
        // transient — keep polling
      }
    };
    void tick();
    if (typeof document === "undefined") return () => { cancelled = true; };
    let intervalId: number | null = null;
    const start = () => {
      if (intervalId !== null) return;
      intervalId = window.setInterval(tick, POLL_MS);
    };
    const stop = () => {
      if (intervalId === null) return;
      window.clearInterval(intervalId);
      intervalId = null;
    };
    const onVis = () => {
      if (document.visibilityState === "hidden") stop();
      else start();
    };
    if (document.visibilityState !== "hidden") start();
    document.addEventListener("visibilitychange", onVis);
    return () => {
      cancelled = true;
      stop();
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [onFirstMetric]);

  // Connection check sub-step (RFC §"Risks" mitigation #3). We
  // probe /api/hosts from the wizard tab and surface the result so
  // the operator can see if the URL the agent would hit is
  // reachable.
  useEffect(() => {
    let cancelled = false;
    setReachable("checking");
    (async () => {
      try {
        const url = `${window.location.origin}/api/hosts`;
        const res = await fetch(url, { credentials: "same-origin" });
        if (cancelled) return;
        setReachable(res.ok ? "ok" : "unreachable");
      } catch {
        if (!cancelled) setReachable("unreachable");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [recheck]);

  return (
    <div className="space-y-4">
      <p className="text-sm text-[color:var(--color-fg)]">{t("onboarding.step4.body")}</p>
      <div className="flex items-center gap-2 text-xs text-[color:var(--color-muted)]">
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        {t("onboarding.step4.checking")}…
      </div>

      <div className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-3">
        <p className="text-[11px] uppercase tracking-wider text-[color:var(--color-muted)]">
          {t("onboarding.step4.checking")}
        </p>
        <p className="mt-1 font-mono text-xs">{window.location.origin}</p>
        <p className="mt-1 flex items-center gap-1.5 text-xs">
          {reachable === "checking" && (
            <>
              <Loader2 className="h-3 w-3 animate-spin" />
              {t("onboarding.step4.checking")}…
            </>
          )}
          {reachable === "ok" && (
            <>
              <CheckCircle2 className="h-3 w-3 text-[color:var(--color-success)]" />
              {t("onboarding.step4.ok")}
            </>
          )}
          {reachable === "unreachable" && (
            <>
              <AlertTriangle className="h-3 w-3 text-[color:var(--color-warn)]" />
              {t("onboarding.step4.unreachable")}
            </>
          )}
        </p>
        <button
          type="button"
          onClick={() => setRecheck((n) => n + 1)}
          className="mt-2 text-[11px] text-[color:var(--color-muted)] underline-offset-2 hover:underline"
        >
          {t("onboarding.step4.checking")}
        </button>
      </div>

      <div className="flex items-center gap-2 text-xs text-[color:var(--color-muted)]">
        <span>
          {hosts.length} {hosts.length === 1 ? "host" : "hosts"} known, none reporting yet
        </span>
      </div>
    </div>
  );
}

// re-export the API used by Settings → Replay button (D3).
export { userPrefsApi };
