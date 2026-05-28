import { createContext, useMemo, useState, type ReactNode } from "react";
import { messages } from "./messages";
import type { Locale, TranslationKey, TranslationParams } from "./types";

const STORAGE_KEY = "lumen.locale";

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey, params?: TranslationParams) => string;
};

export const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(initialLocale);

  const value = useMemo<I18nContextValue>(() => {
    function setLocale(next: Locale) {
      localStorage.setItem(STORAGE_KEY, next);
      setLocaleState(next);
    }

    function t(key: TranslationKey, params?: TranslationParams) {
      const template = lookup(messages[locale], key) ?? lookup(messages.en, key) ?? key;
      return interpolate(template, params);
    }

    return { locale, setLocale, t };
  }, [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

function initialLocale(): Locale {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "en" || stored === "vi") return stored;
  return navigator.language.toLowerCase().startsWith("vi") ? "vi" : "en";
}

function lookup(messagesForLocale: unknown, key: string): string | null {
  let current: unknown = messagesForLocale;
  for (const part of key.split(".")) {
    if (!current || typeof current !== "object" || !(part in current)) return null;
    current = (current as Record<string, unknown>)[part];
  }
  return typeof current === "string" ? current : null;
}

function interpolate(template: string, params?: TranslationParams): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (match, name) => {
    const value = params[name];
    return value === undefined ? match : String(value);
  });
}
