#!/usr/bin/env node
// scripts/i18n-audit.mjs
//
// Sprint 7 / RFC 0007 §"D1 — Parity CI + drift fix".
// Pre-PR / pre-commit helper. Walks `web/src/**/*.{ts,tsx}` and flags
// JSX string literals that look like user-facing copy (≥ 3 ASCII letters)
// and are NOT already inside a `t("...")` call.
//
// Conservative on purpose — false positives are tolerable (operator
// eyeballs + removes), false negatives are not. The mechanical parity
// check (`i18n-parity.mjs`) is the CI gate; this script is the
// pre-PR broom that catches the source-but-not-bundle drift pattern
// that bit us in Sprint 3.
//
// Run from the repo root:
//   node scripts/i18n-audit.mjs
//   # or:
//   pnpm i18n:audit

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, extname } from "node:path";

const ROOT = "web/src";
const EXTS = new Set([".ts", ".tsx"]);
const SKIP_DIRS = new Set(["node_modules", "dist", ".turbo"]);

// JSX text-content heuristic: ≥ 3 ASCII letters, doesn't look like a
// type/import/comment. We deliberately do NOT try to parse JSX — the
// regex below catches the 95% case (literal text between > and <).
const JSX_TEXT_RE = />([^<>{}]{3,})</g;
// Inside-template-literal and prop="..." strings that look like copy.
const STRING_LIT_RE = /["']([A-Z][\w\s.,!?\-:'/()]{4,})["']/g;
// Callsite detector — if a string is the first arg of `t(`, `tn(`,
// `i18n(`, or `formatMessage(`, skip it.
const T_CALL_RE = /\b(t|tn|i18n|formatMessage)\s*\(\s*["']/;

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) {
      if (SKIP_DIRS.has(name)) continue;
      yield* walk(p);
    } else if (EXTS.has(extname(p))) {
      yield p;
    }
  }
}

// Allow-list: brand names, units, common props we don't want to flag.
const ALLOWLIST_PATTERNS = [
  /^(Lumen|EN|VI|N\/A|TODO|FIXME|XXX|WIP|HACK)$/i,
  /^[a-z-]+:\/\//, // URLs
  /^\d+(\.\d+)?(ms|s|m|h|d|kb|mb|gb|tb|%)?$/i, // numeric units
  /^px|rem|em|%$|^#[0-9a-f]{3,8}$/i,
];

function isAllowlisted(s) {
  const t = s.trim();
  if (t.length < 3) return true;
  if (!/[a-zA-Z]/.test(t)) return true;
  return ALLOWLIST_PATTERNS.some((re) => re.test(t));
}

const findings = [];
for (const file of walk(ROOT)) {
  const src = readFileSync(file, "utf8");
  const lines = src.split("\n");

  // Find candidate JSX text content.
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Skip lines that are clearly type/import/comment-only.
    if (/^\s*(\/\/|\*|import |export type |export \{)/.test(line)) continue;
    // Skip if the line contains a t("...") call (covers the common case).
    if (T_CALL_RE.test(line)) continue;

    let m;
    JSX_TEXT_RE.lastIndex = 0;
    while ((m = JSX_TEXT_RE.exec(line)) !== null) {
      const text = m[1].trim();
      if (isAllowlisted(text)) continue;
      findings.push({ file, line: i + 1, text, kind: "jsx" });
    }

    STRING_LIT_RE.lastIndex = 0;
    while ((m = STRING_LIT_RE.exec(line)) !== null) {
      const text = m[1].trim();
      if (isAllowlisted(text)) continue;
      // Skip if the line is `something: "..."` (prop key) — we already
      // caught copy via JSX_TEXT_RE; this catches `defaultMessage="..."`
      // style props. Stay conservative: only flag if the line is a
      // JSX attribute (has `=` + line ends with `/>` or contains `/>`).
      if (!/>|\s*[A-Za-z]+="[^"]*"/.test(line)) continue;
      findings.push({ file, line: i + 1, text, kind: "attr" });
    }
  }
}

if (findings.length === 0) {
  console.log("i18n audit OK — no inline copy detected in JSX text or props.");
  process.exit(0);
}

console.warn(`i18n audit found ${findings.length} candidate(s) — review:`);
for (const f of findings) {
  console.warn(`  ${f.file}:${f.line}  [${f.kind}]  ${JSON.stringify(f.text)}`);
}
process.exit(0); // non-blocking: this script informs, doesn't gate.
