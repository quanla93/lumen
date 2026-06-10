import { lazy, Suspense, useEffect, useState } from "react";
import { authApi, hostsApi, userPrefsApi, ApiError, type User, type Host } from "@/lib/api";
import { LoginForm } from "@/components/LoginForm";
import { RegisterForm } from "@/components/RegisterForm";
import { AppShell, type Tab } from "@/components/AppShell";
import { Dashboard } from "@/components/Dashboard";
import { HostDetailSkeleton } from "@/components/HostDetailSkeleton";
import { Settings } from "@/components/Settings";
import { Alerts } from "@/components/Alerts";
import { CenterCard } from "@/components/CenterCard";
import { PrefsApply } from "@/components/PrefsApply";
import { PrefsProvider } from "@/lib/userPrefs";
import { StatusPage } from "@/components/StatusPage";
import { OnboardingWizard } from "@/components/OnboardingWizard";
import { shouldShowWizard } from "@/lib/hosts";
import { useI18n } from "@/i18n/useI18n";

// HostDetail is the biggest single chunk in the entry bundle
// (react-grid-layout, uPlot, the per-host chart catalog, OKLCH
// helpers). Lazy-load it so the Dashboard's first paint doesn't
// pay for the host detail layer — the user only navigates into
// HostDetail after clicking a card. The Suspense fallback is
// HostDetailSkeleton, which mirrors the page's structure
// (header + metric grid) so the layout doesn't jump.
//
// lazy() expects a module with a default export. HostDetail
// exports a named function, so the .then() adapter re-shapes
// the module into a { default: ... } object that lazy()
// accepts.
const HostDetail = lazy(() =>
  import("@/components/HostDetail").then((m) => ({ default: m.HostDetail })),
);

type View =
  | { kind: "loading" }
  | { kind: "register" }
  | { kind: "login" }
  | { kind: "app"; user: User; tab: Tab; detailHost: string | null };

// Public /status short-circuits the auth bootstrap entirely so an
// unauthenticated visitor never hits /api/setup-status. Plain pathname
// check is intentional — Lumen doesn't ship a router and adding one for
// a single sibling route would be heavier than this branch.
function isPublicStatusRoute() {
  return typeof window !== "undefined" && window.location.pathname.replace(/\/$/, "") === "/status";
}

export default function App() {
  const { t } = useI18n();
  const [view, setView] = useState<View>({ kind: "loading" });

  useEffect(() => {
    if (isPublicStatusRoute()) {
      return;
    }
    bootstrap().then(setView).catch((err) => {
      console.error("bootstrap failed", err);
      setView({ kind: "login" });
    });
  }, []);

  if (isPublicStatusRoute()) {
    return <StatusPage />;
  }

  switch (view.kind) {
    case "loading":
      return (
        <CenterCard title="Lumen">
          <p className="text-sm text-[color:var(--color-muted)]">{t("app.loading")}</p>
        </CenterCard>
      );
    case "register":
      return (
        <RegisterForm
          onSuccess={(user) =>
            setView({ kind: "app", user, tab: "dashboard", detailHost: null })
          }
        />
      );
    case "login":
      return (
        <LoginForm
          onSuccess={(user) =>
            setView({ kind: "app", user, tab: "dashboard", detailHost: null })
          }
        />
      );
    case "app": {
      return (
        <AppView
          user={view.user}
          tab={view.tab}
          detailHost={view.detailHost}
          onHome={() => setView({ ...view, tab: "dashboard", detailHost: null })}
          onTabChange={(tab) => setView({ ...view, tab, detailHost: null })}
          onSelectHost={(name) => setView({ ...view, detailHost: name })}
          onBack={() => setView({ ...view, detailHost: null })}
          onLogout={() => setView({ kind: "login" })}
        />
      );
    }
  }
}

async function bootstrap(): Promise<View> {
  const status = await authApi.setupStatus();
  if (!status.admin_exists) {
    return { kind: "register" };
  }
  try {
    const user = await authApi.me();
    return { kind: "app", user, tab: "dashboard", detailHost: null };
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      return { kind: "login" };
    }
    throw err;
  }
}

// AppView owns the post-auth view shell. It hosts the OnboardingWizard
// (Sprint 5 / RFC 0005) above the AppShell + body. The wizard polls
// the host list and self-closes once the operator has a host that
// reported metrics — at which point the body is just the regular
// Dashboard / Settings / Alerts tab.
function AppView({
  user,
  tab,
  detailHost,
  onHome,
  onTabChange,
  onSelectHost,
  onBack,
  onLogout,
}: {
  user: User;
  tab: Tab;
  detailHost: string | null;
  onHome: () => void;
  onTabChange: (tab: Tab) => void;
  onSelectHost: (name: string) => void;
  onBack: () => void;
  onLogout: () => void;
}) {
  const [hosts, setHosts] = useState<Host[]>([]);
  const [dismissedAt, setDismissedAt] = useState<string | null>(null);
  // Force-mount toggle. Bumping this number tears down + re-mounts
  // the wizard so the "Replay onboarding" button in Settings →
  // Account can clear `dismissedAt` AND have the wizard re-evaluate
  // step visibility from a fresh state on the next render.
  const [replayNonce, setReplayNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const refresh = () => {
      Promise.all([hostsApi.list(), userPrefsApi.get()])
        .then(([list, prefs]) => {
          if (cancelled) return;
          setHosts(list);
          setDismissedAt(prefs.onboarding?.dismissedAt ?? null);
        })
        .catch(() => {});
    };
    refresh();
    const id = window.setInterval(refresh, 5_000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [replayNonce]);

  // The wizard must re-render whenever the operator clicks
  // "Replay" so shouldShowWizard(hosts, null, true) returns true
  // again. We key the wizard on a value that changes only when
  // replayNonce bumps — that way normal hosts/dismissed state
  // updates don't tear down the wizard mid-flow.
  const wizardKey = `wizard-${replayNonce}`;

  let body;
  if (tab === "dashboard") {
    body = detailHost ? (
      <Suspense fallback={<HostDetailSkeleton onBack={onBack} />}>
        <HostDetail hostName={detailHost} onBack={onBack} />
      </Suspense>
    ) : (
      <Dashboard onSelectHost={onSelectHost} onNavigateToSettings={() => onTabChange("settings")} />
    );
  } else if (tab === "alerts") {
    body = <Alerts />;
  } else {
    body = <Settings user={user} onReplayOnboarding={() => setReplayNonce((n) => n + 1)} />;
  }

  return (
    <PrefsProvider>
      <PrefsApply />
      {shouldShowWizard(hosts, dismissedAt, true) && (
        <OnboardingWizard
          key={wizardKey}
          hosts={hosts}
          dismissedAt={dismissedAt}
          onDismissed={async (when) => {
            setDismissedAt(when);
            try {
              await userPrefsApi.putOnboarding({ dismissedAt: when });
            } catch {
              // best-effort: the local state is the source of truth
              // for this session; the next /api/me/prefs will pick
              // it up on the next bootstrap.
            }
          }}
          onHostsChange={setHosts}
        />
      )}
      <AppShell
        user={user}
        tab={tab}
        onTabChange={onTabChange}
        onHome={onHome}
        onLogout={onLogout}
      >
        {body}
      </AppShell>
    </PrefsProvider>
  );
}
