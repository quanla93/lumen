import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
	site: "https://quanla.org",
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
			editLink: {
				baseUrl: "https://github.com/quanla93/lumen/edit/main/docs/",
			},
			lastUpdated: true,
			pagination: true,
			sidebar: [
				{
					label: "Getting started",
					autogenerate: { directory: "getting-started" },
				},
				{
					label: "Install",
					autogenerate: { directory: "install" },
				},
				{
					label: "Configure",
					autogenerate: { directory: "configure" },
				},
				{
					label: "Integrations",
					autogenerate: { directory: "integrations" },
				},
				{
					label: "How-to",
					autogenerate: { directory: "how-to" },
				},
				{
					label: "Reference",
					autogenerate: { directory: "reference" },
				},
				{
					label: "Plugins",
					autogenerate: { directory: "plugins" },
				},
				{
					label: "Contributing",
					autogenerate: { directory: "contributing" },
				},
			],
			customCss: ["./src/styles/custom.css"],
		}),
	],
});
