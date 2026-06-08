/** Typed fetch wrappers for the hub API. All paths are relative so the
 * same client works in `pnpm dev` (Vite proxies to :8090) and in the
 * embedded build (hub serves both /api and the SPA from :8090). */

export type User = {
  id: number;
  username: string;
  created_at: string;
};

export type SystemMetadata = {
  os?: string;
  hostname?: string;
  primary_ip?: string;
  kernel?: string;
  arch?: string;
  cpu_model?: string;
  uptime_seconds?: number;
  agent_version?: string;
  // VirtualizationSystem ("kvm", "lxc", "docker", "wsl", …) when the
  // agent runs in a guest; empty/undefined on bare metal. UI uses this
  // to hide per-core CPU on hosts where the data doesn't isolate.
  virt_type?: string;
};

export type TagFacet = {
  key: string;
  value: string;
  host_count: number;
};

export type Host = {
  id: number;
  name: string;
  created_at: string;
  last_seen_at: string | null;
  system?: SystemMetadata;
  metadata_updated_at?: string;
  tags: Record<string, string>;
  // RFC3339 UTC. Present + future = silence active; absent = none /
  // expired. Backend omits past values, so FE never needs to compare
  // to clock to decide "is the silence still alive".
  silenced_until?: string | null;
  // Whether the host is opted into the public /status page.
  public_visible: boolean;
};

export type CreateHostResponse = {
  host: Host;
  token: string;
};

export type MetricPoint = {
  ts: string;
  cpu_pct: number;
  ram_pct: number;
  swap_pct: number;
  disk_pct: number;
  load1: number;
  load5: number;
  load15: number;
  net_rx_bps: number;
  net_tx_bps: number;
  disk_r_bps: number;
  disk_w_bps: number;
  temp_c: number;
};

export type MetricsResponse = {
  host: string;
  from: string;
  to: string;
  step_seconds: number;
  points: MetricPoint[];
};

export type MetricsQuery = {
  from?: string; // RFC3339
  to?: string;   // RFC3339
  step?: string; // e.g. "30s", "5m"
};

export class ApiError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // body wasn't JSON; keep status text
    }
    throw new ApiError(msg, res.status);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const authApi = {
  setupStatus: () => api<{ admin_exists: boolean; oidc_enabled: boolean; saml_enabled: boolean }>("/api/setup-status"),
  me: () => api<User>("/api/me"),
  register: (username: string, password: string) =>
    api<User>("/api/register", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    api<User>("/api/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  logout: () => api<void>("/api/logout", { method: "POST" }),
  changePassword: (current: string, newPw: string) =>
    api<void>("/api/account/password", {
      method: "POST",
      body: JSON.stringify({ current, new: newPw }),
    }),
};

export type SettingsResponse = {
  retention_window: string;
  retention_interval: string;
  retention_alerts_window: string;
  agent_interval: string;
  downsample_bucket_size: string;
  downsample_hot_window: string;
  downsample_archive_window: string;
};

export type VersionResponse = {
  hub_version: string;
  latest_agent_version: string;
};

export const versionApi = {
  get: () => api<VersionResponse>("/api/version"),
};

// agentUpdateAvailable reports whether a host's reported agent version is
// known, real (not a dev/source build), and behind the latest the hub can
// vouch for. Dev builds on either side suppress the badge to avoid noise.
export function agentUpdateAvailable(
  agentVersion: string | undefined,
  latest: string | undefined,
): boolean {
  if (!agentVersion || !latest) return false;
  if (agentVersion === "dev" || latest === "dev") return false;
  return agentVersion !== latest;
}

export const settingsApi = {
  get: () => api<SettingsResponse>("/api/settings"),
  put: (s: Partial<SettingsResponse>) =>
    api<SettingsResponse>("/api/settings", {
      method: "PUT",
      body: JSON.stringify(s),
    }),
};

// OIDC SSO settings — single-admin mode. expected_email must match the
// IdP's `email` claim exactly; without it, /api/auth/oidc/callback refuses
// any identity. client_secret is write-only over the wire (server stores
// it AES-GCM-encrypted with the hub session secret); has_client_secret
// tells the UI whether to render "saved" vs "unset".
export type OIDCSettings = {
  enabled: boolean;
  issuer: string;
  client_id: string;
  client_secret?: string;
  has_client_secret: boolean;
  scopes: string;
  expected_email: string;
};

export type SAMLSettings = {
  enabled: boolean;
  idp_metadata_xml?: string;
  idp_metadata_url?: string;
  sp_entity_id?: string;
  expected_nameid?: string;
  has_sp_keypair: boolean;
  sp_cert?: string;
  allowed_clock_skew_seconds: number;
  discovered_sso_url?: string;
  discovered_entity_id?: string;
};

export const oidcApi = {
  get: () => api<OIDCSettings>("/api/settings/oidc"),
  put: (s: Partial<OIDCSettings>) =>
    api<OIDCSettings>("/api/settings/oidc", {
      method: "PUT",
      body: JSON.stringify(s),
    }),
  testDiscovery: (issuer: string) =>
    api<{ ok: boolean; error?: string }>("/api/settings/oidc/test", {
      method: "POST",
      body: JSON.stringify({ issuer }),
    }),
};

export const samlApi = {
  get: () => api<SAMLSettings>("/api/settings/saml"),
  put: (s: Partial<SAMLSettings>) =>
    api<SAMLSettings>("/api/settings/saml", {
      method: "PUT",
      body: JSON.stringify(s),
    }),
  testMetadata: (xml: string, url: string) =>
    api<{ ok: boolean; sso_url?: string; idp_entity_id?: string; error?: string }>(
      "/api/settings/saml/test-metadata",
      { method: "POST", body: JSON.stringify({ xml, url }) },
    ),
  metadataUrl: () => "/api/auth/saml/metadata",
};

export type BackupSettings = {
  enabled: boolean;
  target: "local" | "s3";
  local_path: string;
  s3_endpoint: string;
  s3_region: string;
  s3_bucket: string;
  s3_prefix: string;
  s3_access_key: string;
  s3_secret_key?: string;
  has_secret_key: boolean;
  s3_force_path_style: boolean;
  has_passphrase: boolean;
  cron: string;
  retain_last: number;
};

export type BackupEntry = {
  name: string;
  size: number;
  created_at: string;
};

export type BackupRunResult = {
  name: string;
  size_bytes: number;
  duration: number; // ns
  hash: string;
};

export const backupApi = {
  get: () => api<BackupSettings>("/api/settings/backup"),
  put: (s: Partial<BackupSettings>) =>
    api<{ ok: boolean }>("/api/settings/backup", {
      method: "PUT",
      body: JSON.stringify(s),
    }),
  test: () =>
    api<{ ok: boolean; error?: string }>("/api/settings/backup/test", {
      method: "POST",
    }),
  setPassphrase: (passphrase: string) =>
    api<{ ok: boolean }>("/api/backup/passphrase", {
      method: "POST",
      body: JSON.stringify({ passphrase }),
    }),
  runNow: (passphrase: string) =>
    api<BackupRunResult>("/api/backup/run", {
      method: "POST",
      body: JSON.stringify({ passphrase }),
    }),
  list: () => api<{ entries: BackupEntry[] }>("/api/backup/list"),
  restore: (name: string, passphrase: string, force = false) =>
    api<{ name: string; created_at: string; restored_to: string; predecessor: string; size_bytes: number }>(
      `/api/backup/restore/${encodeURIComponent(name)}`,
      {
        method: "POST",
        body: JSON.stringify({ passphrase, force }),
      },
    ),
  downloadUrl: (name: string) => `/api/backup/download/${encodeURIComponent(name)}`,
};

export type HubStatsResponse = {
  version: string;
  started_at: string;
  uptime_seconds: number;
  storage: {
    db_path: string;
    db_size_bytes: number;
    wal_size_bytes: number;
    rows: Record<string, number>;
  };
  runtime: {
    go_version: string;
    goroutines: number;
    heap_alloc_bytes: number;
    num_gc: number;
  };
  agents: {
    connected: number;
    registered: number;
  };
  deliveries: {
    pending: number;
    inflight: number;
  };
};

export const hubStatsApi = {
  get: () => api<HubStatsResponse>("/api/admin/hub-stats"),
};

export type ApiKeyScope = "read:hosts" | "read:metrics" | "read:alerts";

export type ApiKey = {
  id: string;
  name: string;
  preview: string;
  scopes: ApiKeyScope[];
  host_filter: string | null;
  last_used_at: string | null;
  created_at: string;
};

export type ApiKeyCreated = ApiKey & { plaintext: string };

export const apiKeysApi = {
  list: () => api<ApiKey[]>("/api/apikeys"),
  create: (input: { name: string; scopes: ApiKeyScope[]; host_filter: string | null }) =>
    api<ApiKeyCreated>("/api/apikeys", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  remove: (id: string) =>
    api<void>(`/api/apikeys/${encodeURIComponent(id)}`, { method: "DELETE" }),
};

// ─── User prefs (RFC 0002 PR2 Level 3 personalization) ──────────────────

export type SortBy = "name" | "hottest" | "last-seen" | "tag";
export type SortDir = "asc" | "desc";
export type DefaultMetric = "all" | "cpu" | "ram" | "disk";
export type Theme = "system" | "light" | "dark";
export type UnitsMode = "auto" | "binary" | "decimal";
export type ReduceMotion = "system" | "on" | "off";
export type Density = "comfortable" | "compact";

export type SavedView = {
  id: string;
  name: string;
  sortBy: SortBy;
  sortDir: SortDir;
  defaultMetric: DefaultMetric;
  hiddenHostIds: string[];
  tagFilter?: string[];
};

export type ChartLayoutItem = {
  i: string;   // chart catalog ID (e.g. "cpu", "ram", "swap")
  x: number;
  y: number;
  w: number;
  h: number;
};

export type DashboardPrefs = {
  schemaVersion: 1;
  sortBy: SortBy;
  sortDir: SortDir;
  defaultMetric: DefaultMetric;
  hiddenHostIds: string[];
  activeViewId: string | null;
  views: SavedView[];
  // Per-host chart layout for the Host detail page. Key = host name.
  // Server caps map size at 50 entries to bound the JSON blob.
  hostDetailLayouts?: Record<string, ChartLayoutItem[]>;
};

export type DisplayPrefs = {
  schemaVersion: 1;
  theme: Theme;
  language: "en" | "vi";
  units: UnitsMode;
  reduceMotion: ReduceMotion;
  density: Density;
};

export type UserPrefsResponse = {
  dashboard: DashboardPrefs | null;
  display: DisplayPrefs | null;
};

export const DEFAULT_DASHBOARD_PREFS: DashboardPrefs = {
  schemaVersion: 1,
  sortBy: "name",
  sortDir: "asc",
  defaultMetric: "all",
  hiddenHostIds: [],
  activeViewId: null,
  views: [],
};

export const DEFAULT_DISPLAY_PREFS: DisplayPrefs = {
  schemaVersion: 1,
  theme: "system",
  language: "en",
  units: "auto",
  reduceMotion: "system",
  density: "comfortable",
};

export const userPrefsApi = {
  get: () => api<UserPrefsResponse>("/api/me/prefs"),
  putDashboard: (prefs: DashboardPrefs) =>
    api<void>("/api/me/prefs/dashboard", {
      method: "PUT",
      body: JSON.stringify(prefs),
    }),
  putDisplay: (prefs: DisplayPrefs) =>
    api<void>("/api/me/prefs/display", {
      method: "PUT",
      body: JSON.stringify(prefs),
    }),
};

// ----- Alerts (Phase 6 / RFC 0001) -----

export type AlertMetric =
  | "cpu_pct"
  | "ram_pct"
  | "swap_pct"
  | "disk_pct"
  | "load1"
  | "offline"
  // GPU + processes (RFC 0003). The alerts engine fires on the
  // worst-of value across the host's GPUs (multi-GPU fleet).
  | "gpu_util"
  | "gpu_temp"
  | "gpu_mem_pct";

export type MaintenanceWindow = {
  id: number;
  start_at: string;
  end_at: string;
  reason: string;
  scope_tags: Record<string, string>;
  created_at: string;
};

export const maintenanceApi = {
  list: (state?: "active" | "upcoming" | "past" | "all") =>
    api<{ windows: MaintenanceWindow[] }>(
      `/api/maintenance${state ? `?state=${state}` : ""}`,
    ),
  create: (body: { start_at: string; end_at: string; reason: string; scope_tags: Record<string, string> }) =>
    api<{ id: number }>("/api/maintenance", { method: "POST", body: JSON.stringify(body) }),
  update: (id: number, body: { start_at: string; end_at: string; reason: string; scope_tags: Record<string, string> }) =>
    api<{ ok: boolean }>(`/api/maintenance/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  delete: (id: number) =>
    api<{ ok: boolean }>(`/api/maintenance/${id}`, { method: "DELETE" }),
};

export type AlertComparator = "gt" | "lt";
export type AlertSeverity = "info" | "warning" | "critical";

export type AlertRule = {
  id: number;
  name: string;
  metric: AlertMetric;
  comparator: AlertComparator;
  threshold: number;
  for_seconds: number;
  // cooldown_seconds: minimum gap between two firing notifications for
  // the same (rule, host) pair. 0 = no cooldown (default). Suppressed
  // firings also skip the event row, so a flapping rule stays out of
  // both the channel and the history table.
  cooldown_seconds: number;
  host: string;
  host_selector: string;
  severity: AlertSeverity;
  enabled: boolean;
  channel_ids: number[];
  created_at: string;
  updated_at: string;
};

export type AlertRuleWrite = {
  name: string;
  metric: AlertMetric;
  comparator: AlertComparator;
  threshold: number;
  for_seconds: number;
  cooldown_seconds: number;
  host: string;
  host_selector: string;
  severity: AlertSeverity;
  enabled?: boolean;
  // null/undefined → backend leaves links unchanged on UPDATE.
  // Empty array → clear all links → broadcast to every enabled channel.
  channel_ids?: number[];
};

export type ChannelType = "ntfy" | "discord" | "webhook" | "telegram" | "email" | "web_push";

export type ChannelConfig = {
  url?: string;
  topic?: string;
  priority?: string;
  bot_token?: string;
  chat_id?: string;
  parse_mode?: string;
  // email
  smtp_host?: string;
  smtp_port?: number;
  username?: string;
  password?: string;
  from_addr?: string;
  to_addr?: string;
};

export type NotificationChannel = {
  id: number;
  name: string;
  type: ChannelType;
  config: ChannelConfig;
  owner_type: string;
  enabled: boolean;
  min_severity: AlertSeverity;
  created_at: string;
  updated_at: string;
};

export type NotificationChannelWrite = {
  name: string;
  type: ChannelType;
  config: ChannelConfig;
  enabled?: boolean;
  min_severity?: AlertSeverity;
};

// Server-side placeholder for any masked channel secret (telegram bot
// token, email SMTP password). The UI keeps this value in the form's
// matching field on edit; PUTting it back tells the hub to preserve the
// stored secret without retyping. Named TELEGRAM_TOKEN_MASK for legacy
// reasons — same literal handles email password too.
export const TELEGRAM_TOKEN_MASK = "**********";
export const SECRET_MASK = TELEGRAM_TOKEN_MASK;

export type AlertEvent = {
  id: number;
  rule_id: number;
  rule_name: string;
  host: string;
  metric: string;
  severity: AlertSeverity;
  state: "firing" | "resolved";
  value: number;
  message: string;
  started_at: string;
  resolved_at: string | null;
};

export type DeliveryStatus = "pending" | "inflight" | "sent" | "failed" | "dropped";

export type DeliveryView = {
  id: number;
  event_id: number;
  channel_id: number;
  channel_name: string;
  channel_type: string;
  severity: AlertSeverity;
  status: DeliveryStatus;
  attempts: number;
  http_status: number | null;
  error: string | null;
  next_retry_at: string | null;
  payload: unknown;
  created_at: string;
  sent_at: string | null;
};

export const alertsApi = {
  rules: {
    list: () => api<AlertRule[]>("/api/alerts/rules"),
    create: (r: AlertRuleWrite) =>
      api<AlertRule>("/api/alerts/rules", {
        method: "POST",
        body: JSON.stringify(r),
      }),
    update: (id: number, r: AlertRuleWrite) =>
      api<AlertRule>(`/api/alerts/rules/${id}`, {
        method: "PUT",
        body: JSON.stringify(r),
      }),
    remove: (id: number) =>
      api<void>(`/api/alerts/rules/${id}`, { method: "DELETE" }),
  },
  channels: {
    list: () => api<NotificationChannel[]>("/api/alerts/channels"),
    create: (c: NotificationChannelWrite) =>
      api<NotificationChannel>("/api/alerts/channels", {
        method: "POST",
        body: JSON.stringify(c),
      }),
    update: (id: number, c: NotificationChannelWrite) =>
      api<NotificationChannel>(`/api/alerts/channels/${id}`, {
        method: "PUT",
        body: JSON.stringify(c),
      }),
    remove: (id: number) =>
      api<void>(`/api/alerts/channels/${id}`, { method: "DELETE" }),
    test: (id: number) =>
      api<{ ok: boolean }>(`/api/alerts/channels/${id}/test`, {
        method: "POST",
      }),
  },
  events: (state: "firing" | "resolved" | "all" = "all", limit = 100) => {
    const params = new URLSearchParams({ state, limit: String(limit) });
    return api<AlertEvent[]>(`/api/alerts/events?${params.toString()}`);
  },
  deliveries: (filter?: { status?: DeliveryStatus; channel_id?: number; severity?: AlertSeverity; limit?: number }) => {
    const params = new URLSearchParams();
    if (filter?.status) params.set("status", filter.status);
    if (filter?.channel_id) params.set("channel_id", String(filter.channel_id));
    if (filter?.severity) params.set("severity", filter.severity);
    params.set("limit", String(filter?.limit ?? 100));
    return api<DeliveryView[]>(`/api/alerts/deliveries?${params.toString()}`);
  },
  retryDelivery: (id: number) =>
    api<{ status: string }>(`/api/alerts/deliveries/${id}/retry`, { method: "POST" }),
};

export type Tag = {
  key: string;
  description: string;
  values: string[];
  host_count: number;
  rule_count: number;
};

export type TagImpact = {
  host_count: number;
  rule_count: number;
  rule_names: string[];
};

// Web Push — bind a browser to a notification_channels row of type
// 'web_push'. The flow is: load VAPID public key → request browser
// permission → PushManager.subscribe → POST the resulting subscription
// to /api/alerts/web-push/subscribe so the hub can fan out alerts to
// it later. The hub stores one subscription per (channel_id, endpoint)
// so the same browser re-subscribing is a no-op.
export type WebPushSubscription = {
  id: number;
  channel_id: number;
  endpoint: string;
  label: string;
  created_at: string;
};

export const webPushApi = {
  getVAPIDPublicKey: () =>
    api<{ public_key: string; subject: string }>("/api/alerts/web-push/vapid-public-key"),
  putSubject: (subject: string) =>
    api<void>("/api/alerts/web-push/subject", {
      method: "PUT",
      body: JSON.stringify({ subject }),
    }),
  subscribe: (body: { channel_id: number; endpoint: string; p256dh: string; auth: string; label: string }) =>
    api<WebPushSubscription>("/api/alerts/web-push/subscribe", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  listSubscriptions: (channelID: number) =>
    api<WebPushSubscription[]>(`/api/alerts/channels/${channelID}/web-push/subscriptions`),
  deleteSubscription: (id: number) =>
    api<void>(`/api/alerts/web-push/subscriptions/${id}`, { method: "DELETE" }),
};

export const tagsApi = {
  list: () => api<Tag[]>("/api/tags"),
  create: (key: string, description: string, values: string[]) =>
    api<Tag>("/api/tags", {
      method: "POST",
      body: JSON.stringify({ key, description, values }),
    }),
  update: (key: string, description: string) =>
    api<Tag>(`/api/tags/${encodeURIComponent(key)}`, {
      method: "PUT",
      body: JSON.stringify({ description }),
    }),
  remove: (key: string) =>
    api<TagImpact>(`/api/tags/${encodeURIComponent(key)}`, { method: "DELETE" }),
  impact: (key: string) =>
    api<TagImpact>(`/api/tags/${encodeURIComponent(key)}/impact`),
  addValue: (key: string, value: string) =>
    api<Tag>(`/api/tags/${encodeURIComponent(key)}/values`, {
      method: "POST",
      body: JSON.stringify({ value }),
    }),
  removeValue: (key: string, value: string) =>
    api<TagImpact>(
      `/api/tags/${encodeURIComponent(key)}/values/${encodeURIComponent(value)}`,
      { method: "DELETE" },
    ),
  valueImpact: (key: string, value: string) =>
    api<TagImpact>(
      `/api/tags/${encodeURIComponent(key)}/values/${encodeURIComponent(value)}/impact`,
    ),
};

// Public /status page (unauthenticated). The page handler always
// returns 200 with { enabled: false } when the operator hasn't published
// the page, so the frontend renders a deterministic "not published"
// notice instead of branching on HTTP status.
export type PublicStatusHost = {
  name: string;
  state: "up" | "stale" | "down" | "unknown";
  cpu_pct: number;
  ram_pct: number;
  disk_pct: number;
  last_seen_at?: string;
};

export type PublicStatus = {
  enabled: boolean;
  title: string;
  description: string;
  generated_at: string;
  hosts: PublicStatusHost[];
};

export type PublicStatusConfig = {
  enabled: boolean;
  title: string;
  description: string;
};

export const publicStatusApi = {
  // Public: no auth, mounted at /api/public/status.
  getPublic: () => api<PublicStatus>("/api/public/status"),
  // Admin: session-protected config GET/PUT.
  getConfig: () => api<PublicStatusConfig>("/api/settings/public-status"),
  putConfig: (cfg: PublicStatusConfig) =>
    api<PublicStatusConfig>("/api/settings/public-status", {
      method: "PUT",
      body: JSON.stringify(cfg),
    }),
};

export const hostsApi = {
  list: () => api<Host[]>("/api/hosts"),
  create: (name: string) =>
    api<CreateHostResponse>("/api/hosts", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  remove: (id: number) =>
    api<void>(`/api/hosts/${id}`, { method: "DELETE" }),
  rotate: (id: number) =>
    api<{ token: string }>(`/api/hosts/${id}/rotate`, { method: "POST" }),
  setTags: (id: number, tags: Record<string, string>) =>
    api<{ tags: Record<string, string> }>(`/api/hosts/${id}/tags`, {
      method: "PUT",
      body: JSON.stringify({ tags }),
    }),
  tagFacets: () => api<TagFacet[]>("/api/host-tags"),
  silence: (id: number, seconds: number) =>
    api<{ silenced_until: string | null }>(`/api/hosts/${id}/silence`, {
      method: "POST",
      body: JSON.stringify({ seconds }),
    }),
  unsilence: (id: number) =>
    api<void>(`/api/hosts/${id}/silence`, { method: "DELETE" }),
  setPublicVisible: (id: number, public_visible: boolean) =>
    api<void>(`/api/hosts/${id}/public-visible`, {
      method: "PUT",
      body: JSON.stringify({ public_visible }),
    }),
  metrics: (id: number, q?: MetricsQuery) => {
    const params = new URLSearchParams();
    if (q?.from) params.set("from", q.from);
    if (q?.to) params.set("to", q.to);
    if (q?.step) params.set("step", q.step);
    const qs = params.toString();
    return api<MetricsResponse>(
      `/api/hosts/${id}/metrics${qs ? `?${qs}` : ""}`,
    );
  },
};
