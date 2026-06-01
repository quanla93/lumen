import { useI18n } from "@/i18n/useI18n";
import { usePrefs } from "@/lib/userPrefs";

// ThemeToggle flips between light + dark by writing through usePrefs.
// The display.theme value can also be 'system' (set via Settings →
// Display), but the toggle button is binary — clicking it forces an
// explicit light/dark choice.
export function ThemeToggle() {
  const { t } = useI18n();
  const { display, updateDisplay } = usePrefs();
  const effectiveDark =
    display.theme === "dark" ||
    (display.theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  const next = effectiveDark ? "light" : "dark";

  return (
    <button
      type="button"
      onClick={() => { updateDisplay({ ...display, theme: next }).catch(() => {}); }}
      aria-label={effectiveDark ? t("theme.switchToLight") : t("theme.switchToDark")}
      className="inline-flex items-center justify-center rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 text-sm text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)] transition-colors"
    >
      {effectiveDark ? `☀️ ${t("theme.light")}` : `🌙 ${t("theme.dark")}`}
    </button>
  );
}
