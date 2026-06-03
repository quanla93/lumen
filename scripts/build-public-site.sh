#!/usr/bin/env bash
# Build the unified Lumen public site for Cloudflare Workers Static Assets.
#
# Output: ./public-site/
#   ├── index.html          ← landing page (from brand/)
#   ├── logo*.svg, etc.     ← brand assets
#   └── docs/               ← Starlight docs site (built with base="/docs")
#       ├── index.html
#       ├── _astro/         ← Astro asset chunks
#       └── …
#
# Cloudflare runs this as the project's build command (wrangler.toml
# `[build].command`). For local preview after running this script:
#
#   cd public-site && python3 -m http.server 8000
#
# Both paths work:
#   - http://localhost:8000/         → landing
#   - http://localhost:8000/docs/    → Starlight docs

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OUT_DIR="public-site"

echo "▸ Cleaning $OUT_DIR/"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/docs"

# 1. Install + build Starlight docs. `astro build` reads
#    `base: "/docs"` from astro.config.mjs and emits internal links
#    with that prefix. Output lands in docs/dist/.
echo "▸ Installing docs deps"
pnpm install --frozen-lockfile --filter lumen-docs...

echo "▸ Building Starlight docs"
pnpm --filter lumen-docs run build

# 2. Move docs build under public-site/docs/. Astro outputs to
#    docs/dist flat; the URL prefix is purely a routing concern, so we
#    move it into the right directory ourselves.
echo "▸ Staging docs into $OUT_DIR/docs/"
cp -R docs/dist/. "$OUT_DIR/docs/"

# 3. Layer the landing page on top — files in brand/ land at the root
#    of the deploy. brand/index.html wins over the docs index at root.
echo "▸ Staging landing into $OUT_DIR/"
cp -R brand/. "$OUT_DIR/"

# 4. brand/DEPLOY.md is operator-internal — don't ship it.
rm -f "$OUT_DIR/DEPLOY.md"

echo ""
echo "✓ Built $OUT_DIR/ — $(find "$OUT_DIR" -type f | wc -l | tr -d ' ') files"
echo "  landing → $OUT_DIR/index.html"
echo "  docs    → $OUT_DIR/docs/index.html"
