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

export type Host = {
  id: number;
  name: string;
  created_at: string;
  last_seen_at: string | null;
  system?: SystemMetadata;
  metadata_updated_at?: string;
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
};

export const settingsApi = {
  get: () => api<SettingsResponse>("/api/settings"),
  put: (s: Partial<SettingsResponse>) =>
    api<SettingsResponse>("/api/settings", {
      method: "PUT",
      body: JSON.stringify(s),
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
