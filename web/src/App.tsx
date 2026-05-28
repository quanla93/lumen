import { useEffect, useState } from "react";
import { authApi, ApiError, type User } from "@/lib/api";
import { LoginForm } from "@/components/LoginForm";
import { RegisterForm } from "@/components/RegisterForm";
import { AppShell, type Tab } from "@/components/AppShell";
import { Dashboard } from "@/components/Dashboard";
import { HostDetail } from "@/components/HostDetail";
import { Settings } from "@/components/Settings";
import { CenterCard } from "@/components/CenterCard";
import { useI18n } from "@/i18n/useI18n";

type View =
  | { kind: "loading" }
  | { kind: "register" }
  | { kind: "login" }
  | { kind: "app"; user: User; tab: Tab; detailHost: string | null };

export default function App() {
  const { t } = useI18n();
  const [view, setView] = useState<View>({ kind: "loading" });

  useEffect(() => {
    bootstrap().then(setView).catch((err) => {
      console.error("bootstrap failed", err);
      setView({ kind: "login" });
    });
  }, []);

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
      const onHome = () =>
        setView({ ...view, tab: "dashboard", detailHost: null });
      const onTabChange = (tab: Tab) =>
        setView({ ...view, tab, detailHost: null });
      const onSelectHost = (name: string) =>
        setView({ ...view, detailHost: name });
      const onBack = () => setView({ ...view, detailHost: null });

      let body;
      if (view.tab === "dashboard") {
        body = view.detailHost ? (
          <HostDetail hostName={view.detailHost} onBack={onBack} />
        ) : (
          <Dashboard onSelectHost={onSelectHost} />
        );
      } else {
        body = <Settings user={view.user} />;
      }
      return (
        <AppShell
          user={view.user}
          tab={view.tab}
          onTabChange={onTabChange}
          onHome={onHome}
          onLogout={() => setView({ kind: "login" })}
        >
          {body}
        </AppShell>
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
