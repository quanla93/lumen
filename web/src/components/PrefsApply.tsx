import { useEffect } from "react";
import { useI18n } from "@/i18n/useI18n";
import { applyDisplayPrefs, usePrefs } from "@/lib/userPrefs";

// PrefsApply is a sibling component (renders nothing) that bridges
// the prefs context to side-effecting state: DOM theme class, i18n
// locale, and the reduce-motion / density dataset attributes the
// stylesheet keys off.
//
// It also listens to the OS-level dark-mode media query so
// `theme: 'system'` keeps live-toggling without a refresh.
export function PrefsApply() {
  const { display } = usePrefs();
  const { locale, setLocale } = useI18n();

  // Apply theme + reduce-motion + density on every display change.
  useEffect(() => {
    applyDisplayPrefs(display);
  }, [display]);

  // Bridge display.language → I18nProvider. Skipping the write-back
  // path (I18n updating display) — Settings UI owns the single edit
  // path for language, and the LanguageToggle component updates
  // display through usePrefs.
  useEffect(() => {
    if (display.language !== locale) {
      setLocale(display.language);
    }
  }, [display.language, locale, setLocale]);

  // Live-track system theme changes when user picked theme='system'.
  useEffect(() => {
    if (display.theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => applyDisplayPrefs(display);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [display]);

  return null;
}
