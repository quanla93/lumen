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
    <div className="min-h-screen flex flex-col bg-[radial-gradient(circle_at_top_left,color-mix(in_oklch,var(--lumen-teal)_18%,transparent),transparent_34rem)]">
      <header className="sticky top-0 z-10 border-b border-[color:var(--color-border)] bg-[color:var(--color-card)]/90 backdrop-blur">
        <div className="mx-auto max-w-6xl px-4 py-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center justify-between gap-4">
            <h1 className="text-base">
              <LumenWordmark size={24} />
            </h1>
            <div className="flex items-center gap-2 sm:hidden">
              <ThemeToggle />
            </div>
          </div>
          <nav className="flex items-center gap-1 rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-1 shadow-sm">
            <TabButton active={tab === "dashboard"} onClick={() => onTabChange("dashboard")}>
              Dashboard
            </TabButton>
            <TabButton active={tab === "settings"} onClick={() => onTabChange("settings")}>
              Settings
            </TabButton>
          </nav>
          <div className="hidden items-center gap-3 sm:flex">
            <span className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1 text-xs text-[color:var(--color-muted)]">
              signed in as <span className="font-mono text-[color:var(--color-fg)]">{user.username}</span>
            </span>
            <ThemeToggle />
            <button
              type="button"
              onClick={logout}
              className="text-sm rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1.5 hover:bg-[color:var(--color-border)] transition-colors"
            >
              Sign out
            </button>
          </div>
        </div>
      </header>
      <main className="flex-1 mx-auto max-w-6xl w-full px-4 py-6 sm:py-8">{children}</main>
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
    "px-3 py-1.5 text-sm rounded-full transition-colors";
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
