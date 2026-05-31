# RFC 0002 — UI polish + Level 3 personalization (post-v0.4.5)

Status: **Proposal — awaiting sign-off** · Created 2026-05-31

> This RFC covers (a) a visual polish pass to bring Lumen from "MVP-shaped"
> to "finished product", and (b) Level 3 personalization within fixed views
> per `ACTION_PLAN.md:111`. It is split into two shippable PRs so visual
> changes land independently of the new prefs backend.
>
> **Hard ranges** (carried verbatim from ACTION_PLAN decision 2026-05-29):
> - NO drag-drop dashboard grid
> - NO query editor / arbitrary panels
> - KEEP fixed views (Dashboard, Host detail, Settings) — personalize WITHIN them only
> - This is NOT a Grafana dashboard builder

## Context

Lumen v0.4.5 ships a complete monitoring product, but UI quality is uneven:
the dashboard summary strip is dense, host cards mix layout concerns, the
settings tab is a single long form, and there's no place for a user to say
"hide that one noisy host" or "show me only production". ACTION_PLAN notes
this explicitly (`2026-05-27` — *"UI polish is a product requirement, not
cosmetic cleanup"*).

Two adjacent decisions block / shape the scope:
- **Anti-feature**: no dashboard builder. Whatever we add must live *within*
  the existing Dashboard / Host detail / Settings views.
- **Approved Level 3 personalization** (`2026-05-29`): sort + hide hosts,
  default metric choice, 1–2 saved views, theme + units — per-user via
  settings KV.

## Scope

**PR1 — Visual polish (no behavior change, client-only)**
- Design tokens: fonts (Inter Variable + JetBrains Mono Variable), elevation
  scale, motion tokens, status-soft background tints.
- `AppShell`, `Dashboard`, `HostCard`, `HostDetail`, `Settings`, `Alerts`
  layout refresh per mockups below.
- Primitive expansion in `ui.tsx`: `Button`, `IconButton`, `Chip`,
  `SegmentedControl`, `Switch`, `Tooltip`, `Popover`, `NumberInput`, `Tag`.
- "Customize" button on Dashboard appears but is **disabled with a tooltip**
  pointing at the next release. No backend changes.

**PR2 — Level 3 personalization wiring (client + server)**
- New `user_prefs(user_id, key, json_value, updated_at)` table + goose
  migration `0014_user_prefs.sql`.
- Three endpoints: `GET /api/me/prefs`, `PUT /api/me/prefs/dashboard`,
  `PUT /api/me/prefs/display`.
- Client `usePrefs()` hook + Dashboard customize popover wired to it.
- Theme / language / units source-of-truth migrates from `localStorage` to
  the server (seed once from local state to avoid resetting users).
- Settings → Display section becomes the canonical place to change theme /
  language / units (Dashboard customize popover only edits dashboard-scoped
  prefs).

**Deferred to PR3+ / future RFC:**
- Tag-based filter inside views (waits for tag inventory to stabilise from
  Phase 6).
- ⌘K command palette (scaffold UI only in PR1).
- Web Push notification prefs (waits for the Web Push channel itself).
- Per-host *card* customization (e.g. "show temp + load on this host's card
  only"). Out of scope — too close to dashboard builder.

## Style direction

### Palette

Keep the existing OKLCH token system (`web/src/index.css`) — already modern,
already supports light + dark, already brand-correct. Add:

```css
@theme {
  /* Elevation — pre-blended for both themes via shadow stack */
  --shadow-1: 0 1px 2px oklch(0% 0 0 / 4%), 0 1px 1px oklch(0% 0 0 / 6%);
  --shadow-2: 0 4px 6px oklch(0% 0 0 / 4%), 0 2px 4px oklch(0% 0 0 / 8%);
  --shadow-3: 0 12px 24px oklch(0% 0 0 / 8%), 0 4px 8px oklch(0% 0 0 / 12%);

  /* Motion */
  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
  --ease-in:  cubic-bezier(0.4, 0, 1, 1);
  --dur-100: 100ms;
  --dur-150: 150ms;
  --dur-250: 250ms;

  /* Status soft tints — for chip/section backgrounds */
  --bg-ok-soft:     color-mix(in oklch, var(--color-accent) 12%, var(--color-card));
  --bg-warn-soft:   color-mix(in oklch, var(--color-warn)   12%, var(--color-card));
  --bg-danger-soft: color-mix(in oklch, var(--color-danger) 12%, var(--color-card));
}
```

The design-system tool initially proposed dark-only ("Dark Mode OLED"); we
override this — Lumen has shipped both modes since v0.1 and ops users on
daytime shifts need light. Both themes ship together.

### Typography

Adopt explicit web fonts (self-hosted via Fontsource so PWA works offline):

```ts
// vite.config + index.css
fonts: {
  sans: 'Inter Variable',         // UI text, labels
  mono: 'JetBrains Mono Variable' // numeric values, host IDs, agent versions, code
}
```

Why these two:
- **Inter Variable** — purpose-built for UI at small sizes, has tabular
  numerics built-in, OFL-licensed, works offline as Fontsource.
- **JetBrains Mono** — used by every ops-adjacent tool (Grafana, K9s, Lazy*).
  Familiar to the target user. Variable + Fontsource = one file.

Type scale (consistent modular):

| Size | Use |
|---|---|
| 11px | Eyebrow labels, badge text |
| 13px | Secondary text, helper |
| 14px | Body, default |
| 16px | Card titles, form input |
| 20px | Page section titles |
| 24px | Page title (mobile) |
| 32px | Page title (desktop) |

All numeric metric values use `font-variant-numeric: tabular-nums` so a
percentage going 89→100 doesn't shift adjacent layout. New utility:
`.lumen-num` (`font-family: var(--font-mono); font-variant-numeric: tabular-nums;`).

### Iconography

Adopt **Lucide React** (`lucide-react`). MIT, ~1500 icons, consistent
1.5px stroke, tree-shakeable. No emoji as structural icons (current code
mostly already complies; sweep for stragglers).

### Motion

- Hover / focus: `var(--dur-150) var(--ease-out)` on `background-color`,
  `border-color`, `transform`.
- Card lift on hover: `transform: translateY(-1px)` + shadow upgrade
  (`--shadow-1` → `--shadow-2`). Never re-layout neighbors.
- Popover / sheet enter: `var(--dur-250) var(--ease-out)` scale + opacity.
- Status transition (host going stale → offline): brief 600ms color cross-
  fade on the status dot, not an instant snap.
- `prefers-reduced-motion`: disable transforms, keep opacity transitions
  (so state still reads).

### Density

Default density is medium-high. Beszel-style host cards (~140-160 px tall)
on desktop; a tighter "compact" mode is a stretch goal (not in this RFC).
On mobile, host cards collapse to single-column full-width.

### Status semantics

Canonical four states, used consistently across all components:

| State | Dot | Meaning |
|---|---|---|
| `ok` | green, pulsing if WS-live | last tick fresh + all metrics in OK band |
| `warn` | amber | stale tick (`> agent_interval`) OR any metric in warn band |
| `danger` | red | offline (past `OfflineAfter`) OR any metric in danger band |
| `muted` | gray | host record but never received a tick |

## Mockups

### Dashboard (desktop ≥1024px)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ◊ Lumen   Dashboard  Alerts  Settings       [⌘K Search hosts]   EN ▾   ☾   │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Dashboard                                              ● Live (WS)          │
│  Fleet at a glance · 12 hosts · updated 2s ago                               │
│                                                                              │
│  ┌──────┬──────┬──────┬──────┬──────┐                                       │
│  │HOSTS │STALE │HOT   │HOT   │HOT   │      View: Default ▾  [⚙ Customize]   │
│  │ 12   │  0   │CPU   │RAM   │DISK  │      Sort: Name ▾    Show: All ▾      │
│  │11 on │ —    │ 78%  │ 82%  │ 91%  │                                       │
│  │      │      │pve-1 │db-01 │nas-2 │                                       │
│  └──────┴──────┴──────┴──────┴──────┘                                       │
│                                                                              │
│  ┌── pve-1 ──────────────● ok ───┐  ┌── db-01 ──────────────● warn ──┐      │
│  │ Proxmox VE 8.2 · 10.0.0.10   │  │ Ubuntu 22 · 10.0.0.20         │      │
│  │                              │  │                                │      │
│  │ CPU  ▆▆▇▇▇▆▆▇▇▇▆▆▇▇▇    78% │  │ CPU  ▃▃▃▃▃▃▃▃▃▃▃▃▃▃▃    41%   │      │
│  │ RAM  ████████░░░░░░░░    64% │  │ RAM  █████████████░░░    82% ⚠ │      │
│  │ Disk ███████░░░░░░░░░░    52% │  │ Disk ██████░░░░░░░░░░    44%   │      │
│  │                              │  │                                │      │
│  │ ↓ 24 MB/s  ↑ 3 MB/s   1s ago │  │ ↓  8 MB/s  ↑ 1 MB/s   2s ago  │      │
│  └──────────────────────────────┘  └────────────────────────────────┘      │
│  (grid continues: auto-fill, minmax(320px, 1fr))                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Dashboard customize popover (PR2 — disabled stub in PR1)

```
┌─── Customize dashboard ──────────────────────────┐
│                                                  │
│  Sort hosts by                                   │
│    ○ Name (A→Z)          ● Hottest first         │
│    ○ Last seen           ○ Tag                   │
│                                                  │
│  Default metric shown                            │
│    ● CPU + RAM + Disk    ○ CPU only              │
│    ○ RAM only            ○ Disk only             │
│                                                  │
│  Hidden hosts                                    │
│    No hosts hidden.    [+ Hide a host…]          │
│                                                  │
│  Saved views (1 / 5)                             │
│    • Default            (current, system)        │
│    • Production         [Switch] [Edit] [×]      │
│                                                  │
│    [+ Save current as a new view]                │
│                                                  │
│                            [Reset]  [Apply]      │
└──────────────────────────────────────────────────┘
```

### Host detail

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ← Dashboard / pve-1                                                          │
│                                                                              │
│  ● pve-1                                  [Edit tags] [Tokens] [⋯ More]      │
│  Proxmox VE 8.2 · 10.0.0.10 · agent 0.4.5 · uptime 32d 4h                    │
│                                                                              │
│  ┌─ CPU ─────────────────────────┐  ┌─ Memory ─────────────────────────┐    │
│  │  78%        12 cores · 2.4 GHz │  │  64%       15.4 / 24.0 GB        │    │
│  │  ┌──────────────────────────┐ │  │  ┌─────────────────────────────┐ │    │
│  │  │ uPlot · per-core toggle  │ │  │  │ uPlot · used / cached / free│ │    │
│  │  └──────────────────────────┘ │  │  └─────────────────────────────┘ │    │
│  │  [1h] [6h] [24h] [7d]          │  │  [1h] [6h] [24h] [7d]            │    │
│  └────────────────────────────────┘  └──────────────────────────────────┘    │
│                                                                              │
│  ┌─ Disk I/O ──────┐ ┌─ Network ─────┐ ┌─ Load ────────┐ ┌─ Temp ──────┐    │
│  │ R: 12 MB/s      │ │ ↓ 24 MB/s     │ │ 1m   0.45     │ │ 52 °C       │    │
│  │ W:  3 MB/s      │ │ ↑  3 MB/s     │ │ 5m   0.32     │ │ ┌─────────┐ │    │
│  │ ┌────────────┐  │ │ ┌──────────┐  │ │ 15m  0.28     │ │ │ chart   │ │    │
│  │ │ chart      │  │ │ │ chart    │  │ │               │ │ └─────────┘ │    │
│  │ └────────────┘  │ │ └──────────┘  │ │               │ │             │    │
│  └─────────────────┘ └───────────────┘ └───────────────┘ └─────────────┘    │
│                                                                              │
│  ┌─ Containers (8 running, 2 stopped) ──────────────────────────────────┐  │
│  │ NAME            IMAGE             STATE     CPU%   MEM               │  │
│  │ ● postgres      postgres:16       running   12.4   1.2 / 4.0 GB      │  │
│  │ ● nginx         nginx:alpine      running    0.1   18 / 256 MB       │  │
│  │ ● redis         redis:7           running    0.8   95 MB / 1 GB      │  │
│  │ ◌ batch-worker  ghcr.io/…:v3      stopped     —     —                │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Settings (two-pane)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Settings                                                                     │
│                                                                              │
│ ┌─ Sidebar ────────┐  ┌─ Main pane ───────────────────────────────────────┐ │
│ │ ▸ Profile        │  │  Display                                          │ │
│ │ ▸ Display ◀      │  │  ─────────────────────────────────────────────    │ │
│ │ ▸ Hosts          │  │                                                   │ │
│ │ ▸ Alerts         │  │  Theme       ○ System  ● Light  ○ Dark            │ │
│ │ ▸ Runtime        │  │  Language    [ EN ]  [ VI ]                       │ │
│ │ ▸ Retention      │  │  Units       ● Auto  ○ Binary (KiB)  ○ Decimal    │ │
│ │ ▸ Tokens         │  │  Reduce      ○ System  ○ On  ● Off                │ │
│ │ ▸ About          │  │  motion                                           │ │
│ │                  │  │                                                   │ │
│ │                  │  │  Dashboard prefs (sort, hidden hosts, saved views)│ │
│ │                  │  │  are managed from the Dashboard ⚙ button.         │ │
│ └──────────────────┘  └───────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Component-level changes (PR1)

Touch list — every file change traces to a mockup element above.

### `web/src/index.css`
- Add font imports (Inter Variable + JetBrains Mono Variable via Fontsource).
- Add tokens: shadow scale, motion scale, status-soft tints (see Palette
  section above).
- Add `.lumen-num` utility (mono + tabular-nums).
- Keep existing `lumen-status-*` and `lumen-w-*` classes — referenced widely.

### `web/src/components/ui.tsx`
Today exports `Surface`, `StatusPill`, `EmptyState`. Add:

| Primitive | API shape (rough) |
|---|---|
| `Button` | `variant: 'primary' \| 'secondary' \| 'ghost' \| 'danger'`, `size: 'sm' \| 'md'`, `loading?: boolean`, `iconLeft/Right?: ReactNode` |
| `IconButton` | wraps `Button` for icon-only, enforces `aria-label`, min 40×40 |
| `Chip` | `tone: StatusTone`, `size: 'sm' \| 'md'`, optional `onClick` |
| `SegmentedControl` | radio-group styled as connected pills (used for theme/units/time-range) |
| `Switch` | a11y-correct checkbox styled as toggle |
| `Tooltip` | wraps Radix `Tooltip` (already a sensible dep choice; or hand-roll a small version) |
| `Popover` | wraps Radix `Popover` — used by Dashboard customize, host actions menu |
| `NumberInput` | mono input, step buttons, used in Runtime settings |
| `Tag` | tiny pill for host tags — different from `Chip` (no tone, smaller) |

All hit areas ≥40×40. Focus rings use `outline: 2px solid var(--color-accent); outline-offset: 2px`.

### `web/src/components/AppShell.tsx`
- Sticky top bar with `backdrop-filter: blur(8px)` and translucent
  background (`color-mix(in oklch, var(--color-bg) 80%, transparent)`).
- Left cluster: `<Logo />` + product name; clicking goes to dashboard.
- Center cluster: nav links with active-route underline + accent color.
- Right cluster: `⌘K` search trigger (PR1 = scaffold, just opens an empty
  popover) + `LanguageToggle` + `ThemeToggle`.
- Mobile: hamburger that opens a sheet with the same nav items.

### `web/src/components/Dashboard.tsx`
- Summary KPIs (`SummaryCard`) compress to a 5-cell strip; horizontal scroll
  on `<sm`.
- New toolbar row beneath summary: `View ▾` + `Sort ▾` + `Show ▾` +
  `[⚙ Customize]` button. In PR1, opening Customize shows a Popover with
  a "Coming in next release" message + the controls disabled. In PR2 they
  wire to `usePrefs()`.
- Host grid switches to `display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px;`.
- Existing search input becomes ⌘K-driven (Dashboard's local input
  remains as a fallback control, just restyled).

### `web/src/components/HostCard.tsx`
- Title row layout:
  ```
  [status dot] [host name large]              [agent-update ▴] [⋯ menu]
  [os · ip · agent ver — all mono muted ]
  ```
- Metric rows use the `.lumen-num` utility for the percent values; bars
  keep `lumen-w-*` quantization.
- Card border gets `outline: 1px solid transparent` that becomes
  `var(--color-border)` on hover, plus `transform: translateY(-1px)` and
  shadow upgrade. Card stays clickable as a whole.
- Footer row: `↓ ↑` net counters + `updated Ns ago`.
- Pulse animation on status dot when `tone==='ok'` and WS status is
  `connected` (CSS keyframe, respects reduced-motion).

### `web/src/components/HostDetail.tsx`
- Header card: status dot + name large, metadata sub-row, action buttons
  (`Edit tags`, `Tokens`, overflow menu).
- Main grid: 2-up CPU + Memory (large), then 4-up Disk I/O + Network +
  Load + Temp (small). Collapse to 1-col on `<md`.
- Time-range chips use the new `SegmentedControl`.
- Container table: status dot per row, mono for names/images,
  tabular-nums for the numeric cells.

### `web/src/components/Settings.tsx`
- Split into two-pane: left rail nav (sticky), right content pane.
- Sections become standalone components: `<ProfileSection/>`,
  `<DisplaySection/>` (new), `<HostsSection/>`, `<AlertsSection/>` (links
  to the dedicated Alerts page), `<RuntimeSection/>`, `<RetentionSection/>`,
  `<TokensSection/>`, `<AboutSection/>` (new — agent version, build info,
  changelog link).
- Mobile: rail collapses to a `<select>` at the top of the content pane.

### `web/src/components/Alerts.tsx`
- Two columns: rules list (left, filterable) + recent alerts feed (right).
- Severity bar: 2-px wide colored bar on the left edge of each row using
  the status tokens.
- Test-fire button per channel becomes an icon button with confirmation toast.

### Other (smaller passes)
- `Sparkline.tsx`: no structural changes; gets a `tone` prop so it can
  tint the line per status. Already tiny — surgical change.
- `UPlotChart.tsx`: pass a `theme: 'light' \| 'dark'` derived from CSS
  custom properties so uPlot axis lines/text don't read black on dark.
- `LoginForm.tsx` / `RegisterForm.tsx`: re-skin with new primitives; same
  fields, same flow.

## Personalization data model (PR2)

### Server

New migration:

```sql
-- internal/hub/storage/migrations/0014_user_prefs.sql
-- +goose Up
CREATE TABLE user_prefs (
  user_id      INTEGER NOT NULL,
  key          TEXT    NOT NULL,
  json_value   TEXT    NOT NULL,
  updated_at   INTEGER NOT NULL,
  PRIMARY KEY (user_id, key),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE user_prefs;
```

Two well-known keys (others may be added later; reader tolerates unknown):

```ts
// key: "dashboard_prefs"
type DashboardPrefs = {
  schemaVersion: 1;
  sortBy:        'name' | 'hottest' | 'last-seen' | 'tag';
  sortDir:       'asc' | 'desc';
  defaultMetric: 'all' | 'cpu' | 'ram' | 'disk';
  hiddenHostIds: string[];
  activeViewId:  string | null;          // 'default' (system) or a saved view id
  views: Array<{                         // hard cap 5
    id:            string;               // uuid
    name:          string;               // user-given, max 32 chars
    sortBy:        DashboardPrefs['sortBy'];
    sortDir:       DashboardPrefs['sortDir'];
    defaultMetric: DashboardPrefs['defaultMetric'];
    hiddenHostIds: string[];
    tagFilter?:    string[];             // optional, AND
  }>;
};

// key: "display_prefs"
type DisplayPrefs = {
  schemaVersion: 1;
  theme:        'system' | 'light' | 'dark';
  language:     'en' | 'vi';
  units:        'auto' | 'binary' | 'decimal';
  reduceMotion: 'system' | 'on' | 'off';
};
```

Endpoints — bearer-authenticated, scoped to the calling user only:

| Method | Path | Body | Returns |
|---|---|---|---|
| `GET`  | `/api/me/prefs`            | — | `{ dashboard: DashboardPrefs \| null, display: DisplayPrefs \| null }` |
| `PUT`  | `/api/me/prefs/dashboard`  | `DashboardPrefs` | `204` |
| `PUT`  | `/api/me/prefs/display`    | `DisplayPrefs` | `204` |

Server-side validation:
- Reject `views.length > 5`.
- Reject unknown enum values for `sortBy`, `defaultMetric`, etc.
- Reject `hiddenHostIds` entries that aren't valid host UUIDs (cheap query).
- Strip trailing whitespace on view names; enforce 1≤len≤32.

Storage shape on disk: one row per (user, key), `json_value` is the JSON
blob. SQLite JSON1 isn't required — we read/write the whole blob each call.
Last-writer-wins is fine for per-user prefs (only one tab edits a key at a
time in practice).

### Client

New hook in `web/src/lib/usePrefs.ts`:

```ts
export function usePrefs() {
  // React state + fetcher; on mount, GET /api/me/prefs.
  // Returns { dashboard, display, updateDashboard(p), updateDisplay(p) }.
  // updates are optimistic with rollback on PUT failure.
  // Falls back to defaults when server returns null for a key.
}
```

Dashboard customize popover and Settings → Display section both consume
this hook.

### Migration of existing local state (PR2 — one-time, client side)

On first load post-upgrade, if `display_prefs` is `null` server-side and
`localStorage.lumen.theme` exists, seed `DisplayPrefs` from local storage
and PUT it. Same for language. This avoids users having to re-pick theme
after an upgrade. After successful seed, local keys are kept for one
release as a fallback in case of rollback, then removed in the next minor.

### Defaults (server-side fallback when key missing)

```ts
const DEFAULT_DASHBOARD_PREFS: DashboardPrefs = {
  schemaVersion: 1,
  sortBy: 'name', sortDir: 'asc',
  defaultMetric: 'all',
  hiddenHostIds: [], activeViewId: null, views: [],
};
const DEFAULT_DISPLAY_PREFS: DisplayPrefs = {
  schemaVersion: 1,
  theme: 'system', language: 'en',
  units: 'auto', reduceMotion: 'system',
};
```

## Scale considerations

Lumen's target is homelab fleets (10–30 hosts). The personalization
model already mostly solves "lots of hosts" as a UX matter (saved
views narrow the visible set, `sortBy: 'hottest'` surfaces problems
first). This section names the additional hardening for when fleets
push beyond ~50 hosts.

### What breaks first, at what N

| N hosts | First pain point | Fix |
|---|---|---|
| ≤30 | — | none |
| 30–50 | Scroll feels long; dashboard search becomes more useful | Default sort = `hottest`; expose search prominently |
| 50–100 | Grid render lag (React reconciles N cards every WS tick) | **Virtualize** the host grid |
| 100–200 | WS payload size (~N KB / tick) | Subscribe filter (push only hosts in active view) |
| 200+ | Sort/filter latency on every tick | Move sort/filter to a memoized worker, or paginate |

### Concrete plan

**Land in PR1 — Virtualize the host grid**
- Add `@tanstack/react-virtual` (~5 KB; smaller and React-18-correct vs
  `react-window`).
- Cutover at N>50 hosts — below that, render the natural grid (avoids
  flicker / sticky-scroll surprises for the typical homelab).
- One implementation detail: virtualization must coexist with the
  `auto-fill, minmax(320px, 1fr)` grid. Use a row-virtualizer over
  pre-computed columns-per-row from a `ResizeObserver`, not a virtual
  grid library (those don't handle variable column counts well).

**Land in PR2 — Hottest-first default for new accounts at N>20**
- When the user has no saved `sortBy` and the fleet >20 hosts on first
  load, seed `sortBy = 'hottest'` rather than `'name'`. One-time
  decision, recorded in their prefs so they keep control afterwards.

**Stretch / PR3+ — Compact card mode**
- Optional density toggle in display prefs: `density: 'comfortable' | 'compact'`.
  Compact = 80 px tall cards, sparkline removed, percentages only.
  Useful for NOC monitor screens.
- Out of scope for this RFC's two PRs; flagged here so the prefs schema
  doesn't have to break to accommodate it later (the field is reserved
  in `DisplayPrefs` from day one — see schema update below).

**Stretch / PR3+ — Server-side view filter on WS subscribe**
- Today the hub pushes the full snapshot array to every connected
  client. With saved views + tag filters, the client knows it'll
  discard most rows. Push the filter (host_ids OR tag_selector) up to
  the hub via the subscribe frame; let the hub stream only matching
  hosts.
- Touches `internal/hub/stream/handler.go` and the WS frame schema —
  not free. Worth doing once we see a real user with >100 hosts.

### Schema reservation

Add `density` to `DisplayPrefs` from PR2 (default `'comfortable'`), even
though the toggle UI doesn't ship until PR3+:

```ts
type DisplayPrefs = {
  schemaVersion: 1;
  theme:        'system' | 'light' | 'dark';
  language:     'en' | 'vi';
  units:        'auto' | 'binary' | 'decimal';
  reduceMotion: 'system' | 'on' | 'off';
  density:      'comfortable' | 'compact';   // reserved; only 'comfortable' honored in PR2
};
```

Reserving the field now avoids `schemaVersion` bump later.

### What we explicitly are NOT doing for scale

- **No data-decimation client-side**. If we ever need fewer points on
  the wire, that's a hub-side downsample concern (existing
  `internal/hub/storage` downsample policy), not a UI hack.
- **No "Show only top N" arbitrary cap**. Saved views with tag filters
  are the right primitive. A magic "top 20" hides hosts in a way the
  user can't easily reason about.
- **No load-on-scroll / infinite scroll**. The fleet is bounded. The
  user wants to *see* it, not paginate it.

## Definition of done

**PR1**
- All six component files plus `index.css` updated per the touch list.
- Lighthouse a11y score ≥ 95 on Dashboard + Host detail at 375px and 1440px.
- Visual snapshot tests in `web/src/__tests__/` for `HostCard`,
  `SummaryCard`, `AppShell`, `Settings sidebar`.
- Customize button visible but disabled with explanatory tooltip
  (desktop = Popover stub; mobile = bottom sheet stub via `vaul`).
- Host grid virtualized when N>50 via `@tanstack/react-virtual`; below
  that, natural grid.
- New dependencies: `lucide-react`, `@fontsource-variable/inter`,
  `@fontsource-variable/jetbrains-mono`, `vaul`,
  `@tanstack/react-virtual`, and (if not present) `@radix-ui/react-popover`
  + `@radix-ui/react-tooltip` + `@radix-ui/react-switch`
  + `@radix-ui/react-dialog`.

**PR2**
- Migration `0014_user_prefs.sql` applies and rolls back cleanly.
- Three endpoints under `/api/me/prefs` pass integration tests including
  validation rejection cases.
- Dashboard customize popover writes through; reload preserves choices.
- Settings → Display section moves theme/language/units off `localStorage`.
- e2e smoke test: log in, hide a host, save a view, switch view, reload,
  state preserved.
- Docs: new section in `docs/src/content/docs/how-to/use-the-web-ui.md`
  covering customize + saved views.

## Anti-features (explicit non-goals)

These will be rejected in review even if otherwise tempting:

- Drag-drop reordering of host cards (would imply per-host position state
  → close to dashboard builder).
- Add/remove individual panels on Host detail.
- User-defined metrics or computed fields (Grafana territory).
- Per-card visualisation choices ("show line chart instead of bar on this
  host"). Pick one for everyone via `defaultMetric`.
- Sharing saved views between users. Each user's prefs are private; export
  is not in scope.

## Decisions (resolved 2026-05-31)

1. **Inter Variable + JetBrains Mono Variable via Fontsource** — accepted.
   PWA caches forever; first-paint cost is one-time.
2. **Radix UI for Popover / Tooltip / Switch / Dialog** — accepted. A11y
   handled correctly out of the box; small per-component imports.
3. **Mobile customize → bottom sheet** — accepted. Implementation:
   [`vaul`](https://vaul.emilkowal.ski) (~3 KB) for the sheet primitive;
   falls back to Radix Dialog if vaul adds friction. Desktop ≥md keeps
   the Popover anchored to the Customize button.
4. **View cap = 5** — accepted.

## Phasing recap

| PR | Surface | Risk | Rollback |
|---|---|---|---|
| **PR1 — Visual polish** | client only | low (no behavior change) | `git revert` |
| **PR2 — Personalization wiring** | client + server (1 table, 3 endpoints) | low-medium (additive schema) | drop table + revert |

Ship PR1 in a v0.4.x patch (no breaking change). PR2 lands as v0.5.0 work
or v0.4.6 if it's small enough — re-evaluate after PR1 review.
