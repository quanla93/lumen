# Deploying the Lumen landing page

The landing page (`brand/index.html`) is pure static HTML — no build step. Cloudflare Pages is the canonical deployment target. Production URL: <https://lumen.quanla.org/>.

## One-time setup on Cloudflare Pages

1. Cloudflare Dashboard → **Workers & Pages** → **Create application** → **Pages** tab → **Connect to Git**.
2. Pick the `quanla93/lumen` repository (authorise the Cloudflare GitHub app if needed).
3. Set up build:
   - **Production branch:** `main`
   - **Framework preset:** *None* (this is plain HTML)
   - **Build command:** *(leave empty)*
   - **Build output directory:** `brand`
   - **Root directory:** *(leave empty — repository root)*
   - **Environment variables:** *(none)*
4. Save & Deploy. The first build attaches `*.pages.dev` automatically.
5. **Custom domain:** Pages project → **Custom domains** → **Set up a custom domain** → enter `lumen.quanla.org`. Cloudflare adds the CNAME automatically if the apex domain (`quanla.org`) is already on Cloudflare DNS.

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
