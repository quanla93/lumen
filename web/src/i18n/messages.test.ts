// web/src/i18n/messages.test.ts
//
// Sprint 7 / RFC 0007 §"D1 — Parity CI + drift fix".
// Unit-test version of the parity invariant so a future refactor
// (e.g. moving messages into per-locale files) keeps a vitest runnable
// check in lockstep with the standalone scripts/i18n-parity.mjs.
//
// Runs under `pnpm --filter web test` (already wired in CI's lint-web
// job).

import { describe, expect, it } from "vitest";
import { en, vi, messages } from "./messages";

function collectKeys(obj: unknown, prefix = ""): string[] {
  if (!obj || typeof obj !== "object") return [];
  const out: string[] = [];
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) {
      out.push(...collectKeys(v, key));
    } else if (typeof v === "string") {
      out.push(key);
    }
  }
  return out;
}

describe("i18n parity", () => {
  it("exports both en and vi", () => {
    expect(en).toBeDefined();
    expect(vi).toBeDefined();
    expect(messages.en).toBe(en);
    expect(messages.vi).toBe(vi);
  });

  it("en and vi have the same leaf-key set (no drift)", () => {
    const enKeys = new Set(collectKeys(en));
    const viKeys = new Set(collectKeys(vi));
    const missingInVi = [...enKeys].filter((k) => !viKeys.has(k)).sort();
    const missingInEn = [...viKeys].filter((k) => !enKeys.has(k)).sort();
    expect(missingInVi, `keys in en but missing in vi: ${missingInVi.join(", ")}`).toEqual([]);
    expect(missingInEn, `keys in vi but missing in en: ${missingInEn.join(", ")}`).toEqual([]);
  });

  it("no leaf value is empty in either locale", () => {
    const empties: string[] = [];
    for (const k of collectKeys(en)) {
      const v = lookup(en, k);
      if (typeof v === "string" && v.trim() === "") empties.push(`en:${k}`);
    }
    for (const k of collectKeys(vi)) {
      const v = lookup(vi, k);
      if (typeof v === "string" && v.trim() === "") empties.push(`vi:${k}`);
    }
    expect(empties, "empty leaf values").toEqual([]);
  });
});

// Tiny lookup helper — mirrors I18nProvider.lookup but exported for
// this test only.
function lookup(obj: unknown, key: string): unknown {
  let cur: unknown = obj;
  for (const part of key.split(".")) {
    if (!cur || typeof cur !== "object") return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}
