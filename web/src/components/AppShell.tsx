import type { ReactNode } from "react";
import { authApi, type User } from "@/lib/api";
import { ThemeToggle } from "@/components/ThemeToggle";
import { LumenWordmark } from "@/components/Logo";

export type Tab = "dashboard" | "settings";

export function AppShell({
  user,
  tab,
  onTabChange,
  onLogout,
  children,
}: {
  user: User;
  tab: Tab;
  onTabChange: (tab: Tab) => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  async function logout() {
    try {
      await authApi.logout();
    } finally {
      onLogout();
    }
  }

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b border-[color:var(--color-border)] bg-[color:var(--color-card)]">
        <div className="mx-auto max-w-5xl px-4 py-3 flex items-center justify-between gap-4">
          <div className="flex items-center gap-6">
            <h1 className="text-base">
              <LumenWordmark size={22} />
            </h1>
            <nav className="flex items-center gap-1">
              <TabButton active={tab === "dashboard"} onClick={() => onTabChange("dashboard")}>
                Dashboard
              </TabButton>
              <TabButton active={tab === "settings"} onClick={() => onTabChange("settings")}>
                Settings
              </TabButton>
            </nav>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-[color:var(--color-muted)] hidden sm:inline">
              signed in as <span className="font-mono">{user.username}</span>
            </span>
            <ThemeToggle />
            <button
              type="button"
              onClick={logout}
              className="text-sm rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 hover:bg-[color:var(--color-border)] transition-colors"
            >
              Sign out
            </button>
          </div>
        </div>
      </header>
      <main className="flex-1 mx-auto max-w-5xl w-full px-4 py-6 sm:py-8">{children}</main>
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  const base =
    "px-2.5 py-1.5 text-sm rounded-md transition-colors";
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? `${base} bg-[color:var(--color-border)] text-[color:var(--color-fg)]`
          : `${base} text-[color:var(--color-muted)] hover:text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]`
      }
    >
      {children}
    </button>
  );
}
