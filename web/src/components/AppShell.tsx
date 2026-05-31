import { useEffect, useState, type ReactNode } from "react";
import { LayoutDashboard, BellRing, Settings as SettingsIcon, LogOut, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { authApi, type User } from "@/lib/api";
import { ThemeToggle } from "@/components/ThemeToggle";
import { LanguageToggle } from "@/components/LanguageToggle";
import { LogoMark, LumenWordmark } from "@/components/Logo";
import { useI18n } from "@/i18n/useI18n";

export type Tab = "dashboard" | "alerts" | "settings";

const SIDEBAR_COLLAPSED_KEY = "lumen.sidebar.collapsed";

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
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") return false;
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  });

  useEffect(() => {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "0");
  }, [collapsed]);

  async function logout() {
    try {
      await authApi.logout();
    } finally {
      onLogout();
    }
  }

  const items: Array<{ tab: Tab; label: string; icon: typeof LayoutDashboard }> = [
    { tab: "dashboard", label: t("shell.dashboard"), icon: LayoutDashboard },
    { tab: "alerts",    label: t("shell.alerts"),    icon: BellRing },
    { tab: "settings",  label: t("shell.settings"),  icon: SettingsIcon },
  ];

  const sidebarW       = collapsed ? "w-16"     : "w-60";
  const mainMargin     = collapsed ? "md:ml-16" : "md:ml-60";
  const navItemClasses = collapsed
    ? "group flex w-full items-center justify-center rounded-lg px-2 py-2.5 transition-colors"
    : "group flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors";

  return (
    <div className="min-h-dvh bg-[radial-gradient(circle_at_top_left,color-mix(in_oklch,var(--lumen-teal)_18%,transparent),transparent_38rem)]">
      {/* Desktop: fixed left sidebar */}
      <aside
        className={`hidden md:flex fixed inset-y-0 left-0 z-20 ${sidebarW} flex-col border-r border-[color:var(--color-border)] bg-[color:var(--color-card)]/90 backdrop-blur transition-[width] duration-[var(--dur-150)] ease-[var(--ease-out)]`}
      >
        <div className={`flex items-center ${collapsed ? "justify-center px-2" : "px-5"} pt-6 pb-4`}>
          <button
            type="button"
            onClick={onHome}
            className="rounded-lg outline-none transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--color-card)]"
            aria-label={t("shell.backToDashboard")}
          >
            {collapsed ? (
              <span className="text-[color:var(--lumen-teal)]"><LogoMark size={24} /></span>
            ) : (
              <LumenWordmark size={28} />
            )}
          </button>
        </div>
        <nav className={`flex-1 ${collapsed ? "px-2" : "px-3"} py-2`}>
          <ul className="space-y-1">
            {items.map(({ tab: itemTab, label, icon: Icon }) => {
              const active = tab === itemTab;
              return (
                <li key={itemTab}>
                  <button
                    type="button"
                    onClick={() => onTabChange(itemTab)}
                    aria-current={active ? "page" : undefined}
                    aria-label={collapsed ? label : undefined}
                    title={collapsed ? label : undefined}
                    className={`${navItemClasses} ${
                      active
                        ? "bg-[color-mix(in_oklch,var(--lumen-teal)_15%,transparent)] text-[color:var(--color-fg)]"
                        : "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)]/40 hover:text-[color:var(--color-fg)]"
                    }`}
                  >
                    <Icon size={18} strokeWidth={active ? 2.25 : 1.75} className={active ? "text-[color:var(--lumen-teal)]" : ""} />
                    {!collapsed && <span>{label}</span>}
                    {!collapsed && active && (
                      <span aria-hidden className="ml-auto h-1.5 w-1.5 rounded-full bg-[color:var(--lumen-teal)]" />
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        </nav>
        <div className={`border-t border-[color:var(--color-border)] ${collapsed ? "px-2 py-3 space-y-2" : "px-3 py-3 space-y-2"}`}>
          {!collapsed && (
            <>
              <div className="px-2 py-1.5 text-[11px] uppercase tracking-wide text-[color:var(--color-muted)]">
                {t("shell.signedInAs")}
              </div>
              <div className="px-2 truncate lumen-num text-sm font-semibold text-[color:var(--color-fg)]">
                {user.username}
              </div>
            </>
          )}
          <div className={`flex ${collapsed ? "flex-col" : "items-center"} gap-1.5 pt-1`}>
            <LanguageToggle />
            <ThemeToggle />
            <button
              type="button"
              onClick={logout}
              aria-label={t("shell.signOut")}
              title={t("shell.signOut")}
              className={`${collapsed ? "" : "ml-auto"} inline-flex h-9 w-9 items-center justify-center rounded-md text-[color:var(--color-muted)] transition-colors hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-danger)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]`}
            >
              <LogOut size={16} strokeWidth={1.75} />
            </button>
          </div>
          <div className="pt-1">
            <button
              type="button"
              onClick={() => setCollapsed((c) => !c)}
              aria-label={collapsed ? t("shell.expandSidebar") : t("shell.collapseSidebar")}
              title={collapsed ? t("shell.expandSidebar") : t("shell.collapseSidebar")}
              className={`${collapsed ? "mx-auto" : "ml-auto"} flex h-8 w-8 items-center justify-center rounded-md text-[color:var(--color-muted)] transition-colors hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)]`}
            >
              {collapsed ? <PanelLeftOpen size={16} strokeWidth={1.75} /> : <PanelLeftClose size={16} strokeWidth={1.75} />}
            </button>
          </div>
        </div>
      </aside>

      {/* Mobile: top bar */}
      <header className="md:hidden sticky top-0 z-20 border-b border-[color:var(--color-border)] bg-[color:var(--color-card)]/90 backdrop-blur">
        <div className="px-4 py-3 flex items-center justify-between gap-3">
          <button
            type="button"
            onClick={onHome}
            className="rounded-lg outline-none transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]"
            aria-label={t("shell.backToDashboard")}
          >
            <LumenWordmark size={22} />
          </button>
          <div className="flex items-center gap-1">
            <LanguageToggle />
            <ThemeToggle />
            <button
              type="button"
              onClick={logout}
              aria-label={t("shell.signOut")}
              className="inline-flex h-9 w-9 items-center justify-center rounded-md text-[color:var(--color-muted)] transition-colors hover:bg-[color:var(--color-border)] hover:text-[color:var(--color-danger)]"
            >
              <LogOut size={16} strokeWidth={1.75} />
            </button>
          </div>
        </div>
        <nav className="border-t border-[color:var(--color-border)] px-2 py-2 flex items-center gap-1">
          {items.map(({ tab: itemTab, label, icon: Icon }) => {
            const active = tab === itemTab;
            return (
              <button
                key={itemTab}
                type="button"
                onClick={() => onTabChange(itemTab)}
                aria-current={active ? "page" : undefined}
                className={`flex-1 inline-flex flex-col items-center gap-0.5 rounded-md px-2 py-1.5 text-[11px] font-medium transition-colors ${
                  active
                    ? "bg-[color-mix(in_oklch,var(--lumen-teal)_15%,transparent)] text-[color:var(--color-fg)]"
                    : "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)]/40 hover:text-[color:var(--color-fg)]"
                }`}
              >
                <Icon size={16} strokeWidth={active ? 2.25 : 1.75} className={active ? "text-[color:var(--lumen-teal)]" : ""} />
                <span>{label}</span>
              </button>
            );
          })}
        </nav>
      </header>

      <main className={`${mainMargin} px-4 py-6 sm:py-8 md:px-8 transition-[margin] duration-[var(--dur-150)] ease-[var(--ease-out)]`}>
        <div className="mx-auto max-w-[1400px]">{children}</div>
      </main>
    </div>
  );
}
