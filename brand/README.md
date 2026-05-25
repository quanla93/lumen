# Lumen Brand Assets

Authoritative source for the Lumen logo, colors, and usage rules.
All files in this directory are licensed under MIT alongside the rest of the project.

## Concept

The mark is **three concentric arcs emanating from a central dot** — a "pulse of light."
It encodes:

- **Lumen** (light) → arcs spreading outward like illumination.
- **Monitoring / heartbeat** → the layered arcs evoke a pulse signal.
- **Three layers** → the three storage tiers (RAM → SQLite → Parquet) and the three wedges.

The mark is intentionally geometric and abstract — it reads cleanly at 16×16 (favicon) and scales to large display sizes without losing identity.

## Files

| File | Use |
|---|---|
| [`logo-mark.svg`](logo-mark.svg) | Icon-only, brand color (teal #14b8a6). For app icon, light backgrounds. |
| [`logo-monochrome.svg`](logo-monochrome.svg) | Icon-only, uses `currentColor`. For arbitrary tints, single-color print. |
| [`logo.svg`](logo.svg) | Full logo (mark + wordmark). For light backgrounds. |
| [`logo-dark.svg`](logo-dark.svg) | Full logo tuned for dark backgrounds (lighter teal + slate text). |
| [`favicon.svg`](favicon.svg) | Favicon-optimized (rounded square background, simplified to 2 arcs). |

## Colors

| Token | Hex | RGB | Usage |
|---|---|---|---|
| `--lumen-teal-50` | `#f0fdfa` | 240,253,250 | Lightest tint, backgrounds |
| `--lumen-teal-200` | `#99f6e4` | 153,246,228 | Hover states, subtle accents |
| `--lumen-teal-300` | `#5eead4` | 94,234,212 | Primary on dark background |
| `--lumen-teal-500` | `#14b8a6` | 20,184,166 | **Primary brand color** |
| `--lumen-teal-700` | `#0f766e` | 15,118,110 | Pressed states, deep accents |
| `--lumen-teal-900` | `#134e4a` | 19,78,74 | Darkest text-safe teal |
| `--lumen-slate-50` | `#f1f5f9` | 241,245,249 | Text on dark |
| `--lumen-slate-900` | `#0f172a` | 15,23,42 | Dark background, favicon bg |
| `--lumen-slate-950` | `#020617` | 2,6,23 | Deepest dark mode bg |

These map to Tailwind's `teal` and `slate` palettes for consistency with the web UI.

## Typography

The wordmark uses **system UI sans** (`ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, Inter, sans-serif`) at weight 600, letter-spacing -0.5. Choosing system fonts keeps the SVG portable (no embedded font) and lightweight — consistent with the project's lightweight ethos.

For docs and product UI, **Inter** is the preferred web font when available.

## Usage rules

✅ **Do**
- Keep clear-space of at least the height of the central dot around the mark.
- Use the dark variant on backgrounds darker than `--lumen-slate-700`.
- Use the monochrome variant for single-color contexts (print, embroidery, single-color stickers).
- Tile, scale, and rotate the mark for decorative pattern uses.

❌ **Don't**
- Don't recolor the wordmark in a third color outside the palette.
- Don't add shadows, glows, or 3D effects — the brand is flat.
- Don't stretch (preserve aspect ratio).
- Don't combine the mark with another logo without explicit permission.
- Don't use the mark to imply endorsement.

## Tagline

Primary: **Lightweight self-hosted monitoring for homelabs.**
Alt:     **Server monitoring you can run on a Raspberry Pi.**
Wedge:   **Proxmox-native. HTTPS-only. HDD-friendly.**

## Voice

- Direct and concrete. Prefer "60 MB" to "lightweight."
- Honest about limits — the anti-features list is part of the brand.
- Friendly but technical. Assume reader is a homelab operator, not a marketing buyer.
- No corporate buzzwords (no "platform," "ecosystem," "AIOps," "observability solution").

## Credits

Designed in-house as part of the project bootstrap.
