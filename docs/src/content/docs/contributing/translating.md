---
title: Translating Lumen
description: How to add or update a UI translation for the Lumen web app — and what the runtime i18n shape forces you to keep in sync.
sidebar:
  order: 10
---

Lumen's web app ships with **runtime i18n** — strings are loaded as a typed object at build time and switched on-the-fly via the `LanguageToggle` in the top bar, no page reload. As of v0.6.x the app ships **English (EN)** and **Vietnamese (VI)**. Adding a third locale (German, Japanese, …) is a small, well-bounded PR.

This page covers:

1. [Where translations live](#where-translations-live)
2. [Adding a missing key to an existing locale](#fix-or-add-a-key-in-an-existing-locale)
3. [Adding a brand-new locale](#add-a-new-locale)
4. [Interpolation, pluralization, edge cases](#interpolation--pluralization)
5. [How the type system catches incomplete translations](#the-type-system-is-your-co-translator)
6. [Docs site translation (Starlight)](#docs-site-translation-starlight) — separate from the web app

## Where translations live

```
web/src/i18n/
├── messages.ts        ← all strings live here, one big typed object
├── types.ts           ← Locale + TranslationKey types derived from messages.ts
├── I18nProvider.tsx   ← context provider + locale persistence
└── useI18n.ts         ← the `t()` + `locale` hook every component calls
```

The whole locale catalog is a **single TypeScript file** rather than separate JSON. Reason: `TranslationKey` is auto-derived from the EN object (`type TranslationKey = LeafKeys<typeof en>`), so any call to `t("foo.bar")` is type-checked at compile time. Splitting locales across JSON would lose that guarantee.

`messages.ts` is organised by **namespace** (`app.*`, `dashboard.*`, `host.*`, `settings.*`, `alerts.*`, `common.*`, …). When adding a new key, put it under the namespace that owns the feature.

## Fix or add a key in an existing locale

Most translation PRs look like this. To rename "Hosts" → "Servers" in the dashboard tab, say:

1. Open `web/src/i18n/messages.ts`.
2. Find the `en` object → the namespace (`dashboard.*` here).
3. Edit the English string. **Save.**
4. Scroll down to the `vi` object → fix the same key.
5. Run `pnpm --filter web exec tsc --noEmit` from the repo root. If TS errors, you missed the matching `vi` key.
6. Open the page in the dev server (`make dev-hub` + `make dev-web`), flip the language toggle, eyeball both versions.

That's it. No build step, no key generation, no extraction tool.

## Add a new locale

Walk through with **German (`de`)** as a worked example. Same shape for any other locale.

### 1. Extend the `Locale` type

`web/src/i18n/types.ts`:

```diff
- export type Locale = "en" | "vi";
+ export type Locale = "en" | "vi" | "de";
```

### 2. Mirror the `en` object as `de`

In `web/src/i18n/messages.ts`, scroll past the closing `} as const;` of `en` (and past the existing `vi` block). Add a new mirror:

```ts
export const de: WidenStrings<typeof en> = {
  app: {
    loading: "Wird geladen…",
    // … every leaf string from `en` must have a German version here.
  },
  // … fill in every namespace the same way.
};
```

`WidenStrings<typeof en>` is a TypeScript trick that lets the compiler **enforce that every key in `en` exists in your locale**, while still accepting strings of any value (rather than insisting on the exact English literal). If you forget a key, you'll get a `Property 'foo' is missing in type` error — see [The type system is your co-translator](#the-type-system-is-your-co-translator).

### 3. Register the locale

At the bottom of `messages.ts`, add the new export to the messages map:

```diff
- export const messages = { en, vi } as const;
+ export const messages = { en, vi, de } as const;
```

### 4. Add the language toggle entry

`web/src/components/LanguageToggle.tsx` (or wherever `setLocale` is offered) lists the available locales. Add an entry so users can actually pick German.

Same for `web/src/components/Settings.tsx → DisplaySettings → languages segmented control` (around line 1320), if you want the language available in the Settings → Display panel.

```diff
options={[
  { value: "en", label: "EN" },
  { value: "vi", label: "VI" },
+ { value: "de", label: "DE" },
]}
```

### 5. Run typecheck + visual check

```bash
pnpm --filter web exec tsc --noEmit       # 0 errors required
pnpm --filter web run build               # production build sanity
make dev-hub                              # open browser, flip toggle to DE
```

### 6. (Optional) Add a Vietnamese / English-only string note

Some strings are deliberately locale-neutral (e.g. `"EN"`, `"VI"`, `"%"`). Don't translate those — leave them as-is.

## Interpolation & pluralization

### Interpolation

Lumen uses **`{name}` placeholders**, evaluated by the `t()` helper:

```ts
t("host.cores", { count: 8, coreLabel: "cores" })
// → "8 cores"
```

When translating, **preserve `{name}` placeholders exactly** — don't translate them and don't reorder them out of grammar. Position is fine, the helper substitutes by key.

VI example from the existing bundle:

```ts
host: {
  cores: "{count} {coreLabel}",      // EN: same shape
  // …
},
```

### Pluralization

Lumen uses a **passed-in plural form** rather than an ICU plural engine. Components that need plurals pre-pick `singular` vs `plural` based on count and pass the matching label as a param:

```ts
const coreLabel = n === 1 ? t("host.coreSingular") : t("host.corePlural");
t("host.cores", { count: n, coreLabel });
```

When adding a new locale that has more than one plural form (Russian, Polish, Arabic, …), you have two options:

1. **Add extra plural keys** (`coreFew`, `coreMany`) and update the few component call-sites that pick — small, explicit, fits the existing pattern.
2. **Open an RFC** to introduce `Intl.PluralRules` — heavier infrastructure, only worth it if multiple new locales need it.

Default to (1) for the first non-EN/VI locale; (2) is a bridge when the plural complexity actually surfaces.

### Date and number formatting

The current code uses small ad-hoc formatters in `web/src/lib/format.ts` and `web/src/lib/time.ts` (relative-time strings, byte formatters, etc.). Some of those need locale-specific output and accept a `locale` arg today — others don't. If a locale needs different formatting, **add a small per-locale fork inside the formatter** rather than scattering `if (locale === "de")` throughout the component code.

## The type system is your co-translator

The `vi: WidenStrings<typeof en>` declaration ensures that **the VI object has exactly the same key shape as `en`**. If you add a new EN key and forget VI, TypeScript will error:

```
Type '{ … }' is missing the following properties from type
  'WidenStrings<{ readonly app: { … } }>': newKey
```

`pnpm --filter web exec tsc --noEmit` (or `pnpm lint` from the repo root, which runs typecheck across the workspace) is the single check that enforces this. **CI runs it on every PR**, so a PR that misses a VI mirror will be flagged automatically — but running locally is faster.

If you intentionally *want* a string only in EN (very rare — usually for one-off debug builds), the type system won't let you skip the VI side. Either add a VI version, or move that string out of the i18n bundle entirely.

## Docs site translation (Starlight)

The Starlight docs site under `docs/src/content/docs/` is **separate from the web app i18n** — Starlight has its own routing-based i18n.

To translate a docs page to Vietnamese:

1. Copy `docs/src/content/docs/<section>/<page>.md` → `docs/src/content/docs/vi/<section>/<page>.md`.
2. Translate the content. Keep the frontmatter `title:` / `description:` translated too.
3. Cross-reference internal links: `/install/agent-docker/` becomes `/vi/install/agent-docker/` on the VI side.
4. `pnpm --filter lumen-docs run build` validates that all links resolve.

The web app and docs site can ship at different translation completeness levels — they're independent.

## What translators should know

- **Tone**: friendly + concise. "Sign in", not "Please sign into your account at your earliest convenience".
- **Sentence case** for buttons + labels ("Save changes", not "Save Changes"). Keep this even in languages where headline case is the convention — it matches the existing UI rhythm.
- **No emoji-flag-as-locale-name**. The toggle uses `EN` / `VI` / etc., not 🇬🇧 / 🇻🇳. (Flag-for-language is widely considered an anti-pattern — language ≠ country, and ISO codes are unambiguous.)
- **Quotation marks**: use the locale's native marks (`„…"` in German, `« … »` in French, `「…」` in Japanese). Don't force the EN-style `"…"` shape.

## PR checklist

When you open a translation PR:

- [ ] `pnpm --filter web exec tsc --noEmit` passes (TS catches missing keys).
- [ ] `pnpm --filter web run build` builds cleanly.
- [ ] Visual eyeball of every translated string in the dev server, in the target locale.
- [ ] Add a CHANGELOG entry under `## Unreleased` (e.g. `Added German UI translation`).
- [ ] If you added a new locale, mention it in the README "Feature highlights" table under the `UI` row.

A maintainer will follow up with native-speaker review when one is available — translation PRs land **even without a native speaker** since the i18n shape is mechanically enforced; a follow-up polish PR can refine wording later.
