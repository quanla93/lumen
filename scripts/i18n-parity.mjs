#!/usr/bin/env node
// scripts/i18n-parity.mjs
//
// Sprint 7 / RFC 0007 §"D1 — Parity CI + drift fix".
// Fails CI if `web/src/i18n/messages.ts` has any key present in one locale
// (`en` or `vi`) but missing in the other. The TS WidenStrings type
// guarantees the same key shape at build time; this script is the
// mechanical runtime check that catches drift the type system can't
// (e.g. one branch losing a leaf between releases).
//
// Run from the repo root (the `pnpm i18n:check` script wires the
// `--import tsx` flag so Node loads the .ts source via tsx's ESM hook):
//   pnpm i18n:check
//
// Exits 0 on parity, 1 on divergence (with file:line-style diff output).

const { messages } = await import("../web/src/i18n/messages.ts");

function walk(obj, prefix = "") {
  const out = new Set();
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) {
      for (const child of walk(v, key)) out.add(child);
    } else {
      out.add(key);
    }
  }
  return out;
}

const enKeys = walk(messages.en);
const viKeys = walk(messages.vi);

const missingInVi = [...enKeys].filter((k) => !viKeys.has(k)).sort();
const missingInEn = [...viKeys].filter((k) => !enKeys.has(k)).sort();

if (missingInVi.length === 0 && missingInEn.length === 0) {
  console.log(
    `i18n parity OK — ${enKeys.size} keys present in both en and vi`,
  );
  process.exit(0);
}

console.error("i18n parity FAIL:");
if (missingInVi.length) {
  console.error(`  ${missingInVi.length} key(s) in en but missing in vi:`);
  for (const k of missingInVi) console.error(`    + ${k}`);
}
if (missingInEn.length) {
  console.error(`  ${missingInEn.length} key(s) in vi but missing in en:`);
  for (const k of missingInEn) console.error(`    - ${k}`);
}
process.exit(1);
