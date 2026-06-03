# Deploying the Lumen landing page

The landing page (`brand/index.html`) is pure static HTML — no build step. Cloudflare's unified Workers + Pages dashboard reads the `wrangler.toml` at the repo root and serves `./brand/` as static assets. Production URL: <https://lumen.quanla.org/>.

## One-time setup

1. Cloudflare Dashboard → **Workers & Pages** → **Create application** → **Connect to Git**.
2. Pick the `quanla93/lumen` repository (authorise the Cloudflare GitHub app if needed).
3. The connect form pre-fills most fields from `wrangler.toml`. Set the build step manually:
   - **Production branch:** `main`
   - **Builds for non-production branches:** ✓ (preview deploys on PRs)
   - **Build command:** `bash scripts/build-public-site.sh`
   - **Deploy command:** *(leave the auto-prefilled `npx wrangler deploy`)*
   The build script produces `./public-site/` — landing page at root plus Starlight docs at `/docs/` — which `wrangler.toml` `[assets].directory = "./public-site"` then publishes.
4. **Connect**. Cloudflare clones the repo, reads `wrangler.toml`, and publishes `./brand/` as static assets. First deploy attaches `*.workers.dev` automatically.
5. **Custom domain:** project → **Settings** → **Domains & Routes** → **Add** → enter `lumen.quanla.org`. Cloudflare adds the CNAME automatically if the apex domain (`quanla.org`) is already on Cloudflare DNS.

> ℹ️ Older `*.pages.dev` projects (created before the unified Workers + Pages dashboard launched) work the same way conceptually — they don't read `wrangler.toml`, instead expect `Build output directory: brand` on the Pages "Builds" tab. The Workers-with-Assets flow above is the recommended path for new projects.

## What auto-deploys do

- Every push to `main` → production deploy at <https://lumen.quanla.org/>.
- Every PR → preview deploy at `https://<commit-sha>.<project>.pages.dev` (visible as a status check on the PR).
- No rollback button needed — re-deploy by reverting the commit on `main` or by selecting an older deployment in the Cloudflare Pages dashboard ("Deployments" tab → triple-dot menu → **Rollback**).

## Updating content

Just edit `brand/index.html` and merge to `main`. The bilingual EN/VI bundle is inline in the closing `<script>` block — no separate i18n file to keep in sync.

When updating roadmap state (Phase 8 items shipping, Phase 9 items moving), update the three roadmap cards in the EN bundle AND the VI bundle in the same PR.

## What's NOT in the landing page

- **Live demo of the hub itself** — security boundary. The landing page links to GitHub for install instructions; the actual hub runs on the operator's hardware. If a public demo ever ships it lives on a separate subdomain.
- **Analytics** — Cloudflare Pages' built-in Web Analytics (privacy-first, no cookies) is enough; we don't ship Google Analytics or anything similar.
- **Authentication** — the page is fully public. Anything operator-gated lives on the hub UI.

## Operating costs

Cloudflare Pages free tier covers this site comfortably:
- 1 build/minute max (we trigger 1–3 builds/week tops)
- 500 builds/month (we use <20)
- Unlimited bandwidth + requests on the static page
- Free SSL + automatic HTTPS

No paid plan needed unless we add Pages Functions or hit very-high traffic.

## Local preview

```bash
cd brand
python3 -m http.server 8000
# open http://localhost:8000/
```

No bundler, no node_modules. Tailwind v3 Play CDN ships utility classes at runtime; the i18n script is inline. Production bundle is a single HTML file plus the SVG assets in this directory.
