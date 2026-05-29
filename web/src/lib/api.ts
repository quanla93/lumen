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
  setupStatus: () => api<{ admin_exists: boolean }>("/api/setup-status"),
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

// ----- Alerts (Phase 6 / RFC 0001) -----

export type AlertMetric =
  | "cpu_pct"
  | "ram_pct"
  | "swap_pct"
  | "disk_pct"
  | "load1"
  | "offline";

export type AlertComparator = "gt" | "lt";
export type AlertSeverity = "info" | "warning" | "critical";

export type AlertRule = {
  id: number;
  name: string;
  metric: AlertMetric;
  comparator: AlertComparator;
  threshold: number;
  for_seconds: number;
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
  host: string;
  host_selector: string;
  severity: AlertSeverity;
  enabled?: boolean;
  // null/undefined → backend leaves links unchanged on UPDATE.
  // Empty array → clear all links → broadcast to every enabled channel.
  channel_ids?: number[];
};

export type ChannelType = "ntfy" | "discord" | "webhook" | "telegram";

export type ChannelConfig = {
  url?: string;
  topic?: string;
  priority?: string;
  bot_token?: string;
  chat_id?: string;
  parse_mode?: string;
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

// Server-side placeholder for the telegram bot token. The UI keeps this
// value in the form's bot_token field on edit; PUTting it back tells the
// hub to preserve the stored token without retyping.
export const TELEGRAM_TOKEN_MASK = "**********";

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
