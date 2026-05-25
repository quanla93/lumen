import { useEffect, useState } from "react";
import { authApi, ApiError, type User } from "@/lib/api";
import { LoginForm } from "@/components/LoginForm";
import { RegisterForm } from "@/components/RegisterForm";
import { AppShell, type Tab } from "@/components/AppShell";
import { Dashboard } from "@/components/Dashboard";
import { Settings } from "@/components/Settings";
import { CenterCard } from "@/components/CenterCard";

type View =
  | { kind: "loading" }
  | { kind: "register" }
  | { kind: "login" }
  | { kind: "app"; user: User; tab: Tab };

export default function App() {
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
          <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
        </CenterCard>
      );
    case "register":
      return (
        <RegisterForm
          onSuccess={(user) =>
            setView({ kind: "app", user, tab: "dashboard" })
          }
        />
      );
    case "login":
      return (
        <LoginForm
          onSuccess={(user) =>
            setView({ kind: "app", user, tab: "dashboard" })
          }
        />
      );
    case "app":
      return (
        <AppShell
          user={view.user}
          tab={view.tab}
          onTabChange={(tab) => setView({ ...view, tab })}
          onLogout={() => setView({ kind: "login" })}
        >
          {view.tab === "dashboard" ? <Dashboard /> : <Settings />}
        </AppShell>
      );
  }
}

async function bootstrap(): Promise<View> {
  const status = await authApi.setupStatus();
  if (!status.admin_exists) {
    return { kind: "register" };
  }
  try {
    const user = await authApi.me();
    return { kind: "app", user, tab: "dashboard" };
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      return { kind: "login" };
    }
    throw err;
  }
}
