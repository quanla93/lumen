// Host detail dashboard-builder chart catalog. Defines the stable set
// of chart IDs the operator can pick from, plus their default size
// hints when added/reset. Render functions live in HostDetail today
// (C.1 ships the catalog metadata only); HostLayout will wire them up
// to react-grid-layout in C.2.
//
// IDs are STABLE within a major version of dashboard_prefs — removing
// one is a breaking change. New IDs can be added freely; older clients
// just ignore unknown entries.

export type ChartId =
  | "cpu"
  | "cpu-per-core"
  | "ram"
  | "swap"
  | "disk"
  | "disk-io"
  | "network"
  | "load"
  | "temperature"
  | "containers";

// Display labels are looked up via i18n at render time (key
// `host.<id>` for most; see HostDetail's existing translations).
export type ChartMeta = {
  id: ChartId;
  // Grid sizing in the 12-column react-grid-layout system. Heights
  // are in row units (~30 px each by default).
  defaultW: number;
  defaultH: number;
  minW: number;
  minH: number;
  // Whether the chart is renderable depends on host-specific data
  // availability (e.g. temperature only on hosts with sensors,
  // containers only on hosts with Docker). The HostLayout component
  // filters out unavailable charts so the catalog picker doesn't
  // offer charts that would render empty.
  availability:
    | "always"               // every host
    | "on-temp"              // needs temp_c sensor readings
    | "on-docker"            // needs container array on live snapshot
    | "on-bare-metal";       // hide on virtualised guests (per-core cpu)
};

export const CHART_CATALOG: Record<ChartId, ChartMeta> = {
  "cpu":          { id: "cpu",          defaultW: 6,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "cpu-per-core": { id: "cpu-per-core", defaultW: 12, defaultH: 4, minW: 6, minH: 3, availability: "on-bare-metal" },
  "ram":          { id: "ram",          defaultW: 6,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "swap":         { id: "swap",         defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "disk":         { id: "disk",         defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "disk-io":      { id: "disk-io",      defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "network":      { id: "network",      defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "load":         { id: "load",         defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "always" },
  "temperature":  { id: "temperature",  defaultW: 4,  defaultH: 4, minW: 3, minH: 3, availability: "on-temp" },
  "containers":   { id: "containers",   defaultW: 12, defaultH: 6, minW: 6, minH: 4, availability: "on-docker" },
};

// Default layout used by HostLayout (C.2) when the operator has no
// saved layout for a host. Matches the current static visual order:
// CPU + RAM big at top, per-core full-width below, smaller charts in
// a 3-col grid below that, containers full-width at the bottom.
export type LayoutItem = {
  i: ChartId;
  x: number;
  y: number;
  w: number;
  h: number;
};

// All 10 catalog entries participate in the builder grid as of v0.6.1.
// Per-core CPU (live ring buffer) and Containers (live table) gate on
// per-host availability (bare-metal + has Docker respectively) — the
// HostLayout filter drops them from the default layout when absent so
// the grid never reserves space for an empty card.
export const DEFAULT_LAYOUT_LG: LayoutItem[] = [
  { i: "cpu",          x: 0, y: 0,  w: 6,  h: 4 },
  { i: "ram",          x: 6, y: 0,  w: 6,  h: 4 },
  { i: "cpu-per-core", x: 0, y: 4,  w: 12, h: 4 },
  { i: "disk",         x: 0, y: 8,  w: 4,  h: 4 },
  { i: "swap",         x: 4, y: 8,  w: 4,  h: 4 },
  { i: "load",         x: 8, y: 8,  w: 4,  h: 4 },
  { i: "network",      x: 0, y: 12, w: 4,  h: 4 },
  { i: "disk-io",      x: 4, y: 12, w: 4,  h: 4 },
  { i: "temperature",  x: 8, y: 12, w: 4,  h: 4 },
  { i: "containers",   x: 0, y: 16, w: 12, h: 6 },
];

// CATALOG_IDS is the explicit ordered list of catalog IDs — used by
// validators (both client and server) to reject unknown chart IDs in
// a saved layout. Kept in sync with the CHART_CATALOG keys.
export const CATALOG_IDS: ChartId[] = [
  "cpu", "cpu-per-core", "ram", "swap", "disk", "disk-io",
  "network", "load", "temperature", "containers",
];
