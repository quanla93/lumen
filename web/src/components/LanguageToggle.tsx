import { useI18n } from "@/i18n/useI18n";
import { usePrefs } from "@/lib/userPrefs";

// LanguageToggle flips display.language through usePrefs. PrefsApply
// bridges the change to I18nProvider's locale state, so the rest of
// the app stays unchanged.
export function LanguageToggle() {
  const { locale, t } = useI18n();
  const { display, updateDisplay } = usePrefs();
  const nextLocale = locale === "en" ? "vi" : "en";
  const nextLabel = nextLocale === "en" ? t("language.english") : t("language.vietnamese");

  return (
    <button
      type="button"
      onClick={() => { updateDisplay({ ...display, language: nextLocale }).catch(() => {}); }}
      aria-label={t("language.switchTo", { locale: nextLabel })}
      className="inline-flex items-center justify-center rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 text-sm text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)] transition-colors"
    >
      {locale === "en" ? t("language.shortVietnamese") : t("language.shortEnglish")}
    </button>
  );
}
