# RFC 0004 — Host detail dashboard builder (Phase 8, v0.6.0)

Status: **Approved — implementation in flight** · Created 2026-06-01

> Supersedes RFC 0002's anti-feature list **for the Host detail page only**.
> Overrides the 2026-05-29 "Mức 1 LOCKED" decision per the 2026-06-01
> Decisions log entry. Dashboard host-grid keeps fixed views + Mức 3
> sort/hide already shipped in v0.6.0; only Host detail gets the
> add / remove / drag / resize affordance.

## Context

Lumen has shipped per-user personalization (theme/lang/units, Dashboard
sort/hide). On the Host detail page operators want more — *add chart
panels they care about, hide panels they don't, drag to reorder, resize
to match their attention budget*. The 2026-05-29 decision parked this
as Mức 1 LOCKED, requiring a new decision to unlock. The 2026-06-01
Decisions log entry does that.

This RFC sets the implementation boundary so we don't slide into
"rebuild Grafana inside Lumen": **chart catalog stays operator-curated,
no query editor, no custom metrics, no per-host arbitrary panels**.
Operators pick from the catalog, place, hide, resize — but every chart
they can pick is one we've designed and shipped.

## Scope

**In (v0.6.0 ship target):**
- Drag-drop + resize on Host detail via [`react-grid-layout`](https://github.com/react-grid-layout/react-grid-layout) (~50 KB).
- Chart catalog (initial set, 10 entries): CPU summary, CPU per-core, Memory, Swap, Disk, Disk I/O, Network, Load, Temperature, Containers.
- Edit mode: toggle button on the page; in edit mode panels show a remove (×) and resize handle; out of edit mode the layout is static.
- "+ Add chart" button → catalog picker dialog → adds chart with default size.
- Layout saved per (user, host) into `dashboard_prefs.hostDetailLayouts: Record<hostName, ChartLayout[]>` (already in the v0.6.0 schema reservation).
- Responsive: defined `lg` / `md` / `sm` breakpoints; layout auto-translates on resize, can be saved per-breakpoint later if operators ask.
- Reset-to-default button.

**Out (deferred / not happening):**
- Query editor / custom PromQL-style expressions.
- User-defined metrics (alert-derived rates, math between metrics).
- Adding chart types not in the catalog.
- Sharing layouts between users.
- Drag-drop on the Dashboard host grid (anti-feature still holds there).
- Density toggle (`comfortable` / `compact`) — schema-reserved, separate ticket.

## Implementation map

### Sub-phases

| Sub-PR | Scope | Status |
|---|---|---|
| **C.1 — Foundation** | Add `react-grid-layout` dep; build the chart catalog (`web/src/components/hostCharts/catalog.ts`) with 10 IDs + per-id default size + availability gate. Per-core CPU rewritten as live uPlot ring buffer. Swap chart added. | ✅ Shipped 2026-06-01 (v0.6.0) |
| **C.2 — Persistence** | Extend `dashboard_prefs` JSON shape with `hostDetailLayouts`; server validator caps 50 hosts × 20 charts × catalog-whitelist; client reads on mount, falls back to default; saves on change (debounced 500ms). | ✅ Shipped 2026-06-01 (v0.6.0) |
| **C.3 — Edit mode + add/remove** | Edit Layout toggle, × on each card, Add chart toggle-switch picker (replaces add/remove split — single picker shows all available with on/off switch), Reset to defaults, **Auto-arrange** (greedy first-fit pack, leftward + upward), smart placement on add (first empty slot) + auto-heal on remove. | ✅ Shipped 2026-06-01 (v0.6.0) |
| **C.4 — Polish (partial)** | i18n EN + VI ✅, README anti-feature rewrite ✅, RFC 0002 marked superseded for Host detail ✅. Breakpoint layouts beyond `lg` deferred — `react-grid-layout` does sensible reflow at narrower widths out of the box for v0.6.0; revisit if operator complains. | Partial — full breakpoint matrix deferred to v0.6.x |

### Files

- `web/package.json` — add `react-grid-layout`, `@types/react-grid-layout`.
- `web/src/components/hostCharts/` — new directory; one file per chart type, each rendering a self-contained `<HostChart cardId="..." snapshot={...}>` block (header + chart + footer).
- `web/src/components/HostDetail.tsx` — refactor: render `<HostLayout>` instead of hard-coded chart blocks.
- `web/src/components/HostLayout.tsx` — new; wraps `react-grid-layout`, reads from `usePrefs().dashboard.hostDetailLayouts[host]`, persists on change.
- `web/src/lib/api.ts` — extend `DashboardPrefs` type with `hostDetailLayouts`.
- `internal/hub/userprefs/userprefs.go` — extend `DashboardPrefs` struct + `ValidateDashboard` to accept the new field. Cap on saved-host-count (50?) to bound blob size.
- `web/src/i18n/messages.ts` — edit-mode button label, add/remove confirmation, chart catalog display names, reset button.

### Chart catalog shape

```ts
// web/src/components/hostCharts/catalog.ts
export type ChartId =
  | "cpu" | "cpu-per-core" | "memory" | "swap"
  | "disk" | "disk-io" | "network" | "load"
  | "temperature" | "containers";

export type ChartEntry = {
  id: ChartId;
  defaultW: number;       // grid columns at lg breakpoint (out of 12)
  defaultH: number;       // grid rows (each row ~30 px)
  minW: number;
  minH: number;
  render: (props: HostChartProps) => ReactNode;
};

export const CHART_CATALOG: Record<ChartId, ChartEntry> = { ... };

export const DEFAULT_LAYOUT_LG: LayoutItem[] = [
  { i: "cpu",          x: 0, y: 0, w: 6, h: 6 },
  { i: "memory",       x: 6, y: 0, w: 6, h: 6 },
  { i: "cpu-per-core", x: 0, y: 6, w: 12, h: 4 },
  { i: "disk",         x: 0, y: 10, w: 3, h: 4 },
  { i: "disk-io",      x: 3, y: 10, w: 3, h: 4 },
  { i: "network",      x: 6, y: 10, w: 3, h: 4 },
  { i: "load",         x: 9, y: 10, w: 3, h: 4 },
  { i: "temperature",  x: 0, y: 14, w: 4, h: 4 },
  { i: "containers",   x: 4, y: 14, w: 8, h: 6 },
];
```

### `dashboard_prefs.hostDetailLayouts` shape (extension of v0.6.0 schema)

```ts
type HostDetailLayout = {
  charts: ChartId[];                 // which catalog entries are visible
  positions: LayoutItem[];           // react-grid-layout shape per item
};

type DashboardPrefs = {
  // ... existing ...
  hostDetailLayouts?: Record<string, HostDetailLayout>;  // key = host name
};
```

Server-side validation (`internal/hub/userprefs/userprefs.go`):
- `hostDetailLayouts` keys: max 50 hostnames per user (prevents unbounded growth).
- `charts`: every entry must be a known catalog ID (server keeps a copy of the catalog ID list for validation).
- `positions`: every `i` field must appear in `charts`; `x/y` non-negative; `w/h` within `[minW..12]` / `[minH..20]`.

## Decisions (resolved 2026-06-01)

1. **`react-grid-layout` over `gridstack.js` or hand-rolled** — accepted. RGL has the best React integration, 50 KB gzip is acceptable, MIT licensed. `gridstack.js` is vanilla-JS-first and would need adapter wrapping; hand-rolled drag-drop is six months of edge cases we don't need to live.
2. **Per-host layout, not global** — accepted. Operators monitor different host types (Proxmox node vs LXC vs bare-metal NAS) and want different layouts. Storing per-host is the right shape; the schema cap (50 hosts/user) keeps blob size bounded.
3. **Edit mode toggle, not always-on drag** — accepted. Always-on drag means panels move when the user scrolls or mistapps. Toggle button → explicit intent.
4. **Default layout matches the current static order** — accepted. Existing operators get the same first-paint they're used to; only operators who edit see a different layout.
5. **Catalog locked at 10 charts for v0.6.0** — accepted. Containers / temp / load already exist in the codebase; the catalog is just naming them and giving each an `id` + render function. Future charts (derived rates, ZFS pool, PBS backup status) come behind future RFCs.

## Anti-features (still locked)

These will be rejected even if otherwise tempting:

- **Query editor** — Lumen is not Prometheus / Grafana. If an operator wants arbitrary queries, they use the Public Read API and Grafana.
- **User-defined metrics / math** — same reason.
- **Adding chart types not in the catalog** — operators can vote for new catalog entries via issues; we ship them in patch releases.
- **Dashboard (host grid) drag-drop** — host cards stay fixed-views + Mức 3 sort/hide.
- **Sharing layouts between users** — each user's prefs are private. Export is not in scope.

## Stability promise

The chart catalog `ChartId` enum is **stable within a major version**. Removing an ID is a breaking change requiring a `v2` of `dashboard_prefs`. Adding new IDs is non-breaking (older clients ignore unknown IDs).

`react-grid-layout`'s position model (`x/y/w/h` 12-column grid) is the de-facto standard; any layout export would round-trip into it. No vendor lock.

## Implementation order (this RFC binds the order)

Ship C.1 → C.2 → C.3 → C.4 strictly in order — each sub-PR is testable on its own without the next one shipping.

- After C.1: HostDetail still renders the same charts, just refactored into catalog primitives. No behavior change.
- After C.2: Layout persists across reloads, but operator can't *edit* yet — they get the default layout regardless.
- After C.3: Operator can edit. Responsive may still be janky at narrow widths.
- After C.4: Full feature complete. v0.6.0 ships.

## Open questions

- Should the chart catalog dialog (C.3) include a search box? At 10 entries it's optional; revisit if catalog grows past 20.
- Should we offer a "preview" hover state in the catalog picker showing the chart at default size? Defer to UX feedback after C.3 ships.
- Mobile: edit mode realistically lives on desktop only. We will hide the "Edit layout" button on `<sm` widths and let mobile users live with whatever desktop saved. Revisit if a real mobile pain surfaces.
