import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
	// Docs are served at lumen.quanla.org/docs/ alongside the landing
	// page at lumen.quanla.org/. The `base` prefix tells Starlight to
	// emit all routes, asset URLs, and internal links with /docs/ in
	// front so the unified Cloudflare deploy serves them correctly.
	site: "https://lumen.quanla.org",
	base: "/docs",
	// The /docs/ root used to render a splash page (template:splash) whose
	// CTAs duplicated the landing page's. A user clicking "Docs" then had to
	// click again to reach actual content. Redirect /docs/ straight to the
	// first real page so the splash never gets in the way. The redirect
	// emits a static HTML meta-refresh page that the Cloudflare assets
	// host serves before the SPA fallback fires.
	redirects: {
		// Destination is written verbatim into the meta-refresh — Astro does
		// not prepend `base` here. Include /docs/ explicitly so the redirect
		// lands inside the docs site instead of jumping to the landing tree.
		"/": "/docs/getting-started/overview/",
	},
	integrations: [
		starlight({
			title: "Lumen",
			description:
				"Lightweight self-hosted monitoring for homelabs. Proxmox-native, HDD-friendly, HTTPS-only.",
			logo: {
				src: "./src/assets/logo.svg",
				replacesTitle: false,
			},
			favicon: "/favicon.svg",
			social: {
				github: "https://github.com/quanla93/lumen",
				// discord: pending — channel not registered yet (pre-v0.1).
			},
			defaultLocale: "root",
			locales: {
				root: { label: "English", lang: "en" },
				vi: { label: "Tiếng Việt", lang: "vi" },
			},
			components: {
				// Override so the header logo + title link to the landing root
				// (/) instead of the docs root (/docs/). Operators read the docs,
				// then click the logo to leave for the landing page.
				SiteTitle: "./src/components/SiteTitle.astro",
			},
			editLink: {
				baseUrl: "https://github.com/quanla93/lumen/edit/main/docs/",
			},
			lastUpdated: true,
			pagination: true,
			// Sidebar trimmed: dropped "Integrations" and "Plugins" (their
			// directories don't exist yet; autogenerate rendered empty
			// groups). The five remaining sections follow the operator
			// journey end-to-end without forcing tab jumps for one task.
			sidebar: [
				{
					label: "Get started",
					autogenerate: { directory: "getting-started" },
				},
				{
					label: "Install",
					autogenerate: { directory: "install" },
				},
				{
					label: "How-to",
					autogenerate: { directory: "how-to" },
				},
				{
					label: "Configure",
					autogenerate: { directory: "configure" },
				},
				{
					label: "Reference",
					autogenerate: { directory: "reference" },
				},
				{
					label: "Contribute",
					autogenerate: { directory: "contributing" },
				},
			],
			customCss: ["./src/styles/custom.css"],
		}),
	],
});
