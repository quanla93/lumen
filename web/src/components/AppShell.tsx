import type { ReactNode } from "react";
import { authApi, type User } from "@/lib/api";
import { ThemeToggle } from "@/components/ThemeToggle";
import { LanguageToggle } from "@/components/LanguageToggle";
import { LumenWordmark } from "@/components/Logo";
import { useI18n } from "@/i18n/useI18n";

export type Tab = "dashboard" | "settings";

export function AppShell({
  user,
  tab,
  onTabChange,
  onHome,
  onLogout,
  children,
}: {
  user: User;
  tab: Tab;
  onTabChange: (tab: Tab) => void;
  onHome: () => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  const { t } = useI18n();

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
            <button
              type="button"
              onClick={onHome}
              className="rounded-lg text-base outline-none transition-transform hover:-translate-y-0.5 focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--color-card)]"
              aria-label={t("shell.backToDashboard")}
            >
              <LumenWordmark size={24} />
            </button>
            <div className="flex items-center gap-2 sm:hidden">
              <LanguageToggle />
              <ThemeToggle />
              <button
                type="button"
                onClick={logout}
                className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1.5 text-xs transition-colors hover:bg-[color:var(--color-border)]"
              >
                {t("shell.signOut")}
              </button>
            </div>
          </div>
          <nav className="flex items-center gap-1 rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-1 shadow-sm">
            <TabButton active={tab === "dashboard"} onClick={() => onTabChange("dashboard")}>
              {t("shell.dashboard")}
            </TabButton>
            <TabButton active={tab === "settings"} onClick={() => onTabChange("settings")}>
              {t("shell.settings")}
            </TabButton>
          </nav>
          <div className="hidden items-center gap-3 sm:flex">
            <span className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1 text-xs text-[color:var(--color-muted)]">
              {t("shell.signedInAs")} <span className="font-mono text-[color:var(--color-fg)]">{user.username}</span>
            </span>
            <LanguageToggle />
            <ThemeToggle />
            <button
              type="button"
              onClick={logout}
              className="text-sm rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-1.5 hover:bg-[color:var(--color-border)] transition-colors"
            >
              {t("shell.signOut")}
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
