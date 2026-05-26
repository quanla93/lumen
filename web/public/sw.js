// Lumen service worker.
//
// Scope: keep the dashboard usable when the network glitches (Wi-Fi
// roam, ISP blip) and let the browser install the app to a homescreen
// icon. NOT trying to be a full offline-first app — live metrics
// fundamentally require network reachability to the hub.
//
// Strategy:
//   • App shell (HTML, JS, CSS) → cache-first with a network-fallback
//     revalidate. Lets the dashboard paint instantly on cold start
//     even before the WS reconnects.
//   • /api/*                    → network-only, never cached. A cached
//     metric snapshot would be misleading; if the network is down,
//     the UI already shows the WS disconnected state.
//
// Cache name is suffixed with a version. Bump on bundle changes so
// older clients drop the stale shell. Vite's content-hashed asset
// names handle JS/CSS invalidation; the cache version covers
// index.html itself.

const CACHE = "lumen-shell-v1";

// Minimal pre-cache: just the entry HTML and icons. Vite-generated
// /assets/*.{js,css} are picked up lazily as the user navigates and
// the shell loads them. Pre-caching them by name would require build
// integration we don't have today.
const SHELL = ["/", "/favicon.svg", "/icon-192.svg", "/icon-512.svg", "/manifest.webmanifest"];

self.addEventListener("install", (event) => {
	event.waitUntil(
		caches.open(CACHE).then((cache) =>
			// addAll is atomic — if any URL fails the install rejects.
			// We list only paths that exist in /public so this is safe.
			cache.addAll(SHELL),
		),
	);
	// New worker takes over immediately on next page load. Without
	// skipWaiting the user would stay on the previous SW until every
	// tab closes — annoying on a homelab dashboard you leave open.
	self.skipWaiting();
});

self.addEventListener("activate", (event) => {
	// Drop any older cache versions.
	event.waitUntil(
		caches.keys().then((keys) =>
			Promise.all(
				keys
					.filter((k) => k.startsWith("lumen-shell-") && k !== CACHE)
					.map((k) => caches.delete(k)),
			),
		),
	);
	self.clients.claim();
});

self.addEventListener("fetch", (event) => {
	const req = event.request;

	// Only GET — POST/PUT/DELETE go straight through. The hub's
	// /api/account/password etc. would break otherwise.
	if (req.method !== "GET") return;

	const url = new URL(req.url);

	// Same-origin only — never proxy cross-origin requests (e.g. a
	// future CDN-hosted font). Lets the browser handle CORS normally.
	if (url.origin !== self.location.origin) return;

	// Live data: never serve from cache.
	if (url.pathname.startsWith("/api/") || url.pathname === "/healthz") {
		return; // browser does the network fetch directly
	}

	// App shell: cache-first, then network. We don't await the
	// background revalidate — the user gets the cached shell now and
	// fresher bits on the next reload.
	event.respondWith(
		caches.match(req).then((cached) => {
			const fetchPromise = fetch(req)
				.then((resp) => {
					// Only cache OK 200 same-origin responses. Skip
					// redirects (304 etc.) — they confuse caches.match.
					if (resp && resp.status === 200 && resp.type === "basic") {
						const clone = resp.clone();
						caches.open(CACHE).then((c) => c.put(req, clone));
					}
					return resp;
				})
				.catch(() => cached); // offline: return whatever we have
			return cached || fetchPromise;
		}),
	);
});
