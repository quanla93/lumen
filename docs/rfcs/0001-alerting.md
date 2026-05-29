# RFC 0001 — Alerting & notifications (Phase 6, v0.5)

Status: **Planned / ready to implement** · Created 2026-05-29

> This is an implementation plan to be executed (possibly on another machine). It is self-contained: it names exact files to create/modify and the existing patterns to reuse. Phase 5 (Proxmox) was deferred to ~v1; Alerting is the next phase (see ACTION_PLAN Decisions log 2026-05-29).

## Context

Lumen monitors hosts but cannot tell you when something breaks — today it's "graphs you have to stare at". Alerting is table-stakes for a monitoring tool (Beszel has it) and is the highest day-to-day-value feature now that Proxmox is deferred. This phase adds threshold-based rules over **any** host/metric, a background evaluation engine with firing/resolved state, and outbound notification channels.

Alerting is general (every host), not Proxmox-specific. Notification channels here are **admin-owned**; the same `notification_channels` table is intended to be reused later by the Public API customer webhooks (owner_type column included now for forward-compat — see Public API module + Decisions log 2026-05-29 webhook decision).

## Scope

**Milestone A (this RFC — first testable):**
- Rules: threshold on a metric (`cpu_pct`/`ram_pct`/`swap_pct`/`disk_pct`/`load1`) with comparator + "for" duration; plus an `offline` rule (host stopped sending ticks); host filter (one host or all).
- Engine: background goroutine, evaluates the in-memory store every ~15s, per-(rule,host) state machine (pending → firing → resolved honouring "for"), persists alert events, dispatches notifications on transitions.
- Channels: **ntfy + Discord + generic webhook** (all are HTTP POST — cheap to do together). ntfy is the easiest to test (no setup).
- UI: a new top-level **Alerts** tab (active alerts + recent + rules CRUD + channels CRUD + Test).
- Docs: `configure/alerts.md`.

**Deferred to Milestone B+:** Email (SMTP), Telegram (bot token); per-rule channel routing + severity→channel filters; cooldown/flap-suppression tuning; alert on derived/rate metrics; alert history retention sweep; HMAC signing on the webhook channel (lands with the Public API webhook unification).

## Backend

### Migration `internal/hub/storage/migrations/0008_alerts.sql`
Mirror the goose style of `0005_settings.sql` (`-- +goose Up` / `-- +goose Down`). Next free number is **0008** (latest is `0007_host_network_metadata.sql`).

```sql
-- +goose Up
CREATE TABLE alert_rules (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  metric      TEXT NOT NULL,                       -- cpu_pct|ram_pct|swap_pct|disk_pct|load1|offline
  comparator  TEXT NOT NULL DEFAULT 'gt',          -- gt|lt (ignored for 'offline')
  threshold   REAL NOT NULL DEFAULT 0,
  for_seconds INTEGER NOT NULL DEFAULT 0,          -- breach must persist this long before firing
  host        TEXT,                                -- NULL = all hosts
  severity    TEXT NOT NULL DEFAULT 'warning',     -- info|warning|critical
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE notification_channels (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  type        TEXT NOT NULL,                        -- ntfy|discord|webhook
  config      TEXT NOT NULL,                        -- JSON: {"url":...,"topic":...,"priority":...}
  owner_type  TEXT NOT NULL DEFAULT 'admin',        -- fwd-compat: 'admin' | (future) 'api_key'
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE alert_events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_id     INTEGER NOT NULL,
  rule_name   TEXT NOT NULL,                        -- denormalized: history survives rule deletion
  host        TEXT NOT NULL,
  metric      TEXT NOT NULL,
  severity    TEXT NOT NULL,
  state       TEXT NOT NULL,                        -- firing|resolved
  value       REAL,
  message     TEXT NOT NULL,
  started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at DATETIME
);
CREATE INDEX idx_alert_events_state ON alert_events(state, started_at);

-- +goose Down
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS alert_rules;
```

### New package `internal/hub/alerts/`
- **`rules.go`** — `Rule` struct + DB CRUD (`List/Get/Create/Update/Delete`). Validate `metric`, `comparator`, `severity` against allowed enums; `for_seconds >= 0`. Mirror the DB-access style of `internal/hub/hosts/hosts.go` (scan helper, `isUniqueViolation` not needed here).
- **`channels.go`** — `Channel` struct + CRUD. `config` is opaque JSON validated per `type` (ntfy needs `url`; discord needs `url`; webhook needs `url`). Never log the full config (URLs can embed tokens).
- **`notify.go`** — `Notification` struct `{RuleName, Host, Metric, Severity, State, Value, Threshold, Message, Time}` and `Dispatch(ctx, ch Channel, n Notification)`:
  - `ntfy`: `POST <url>` (e.g. `https://ntfy.sh/<topic>` — `url` holds the full topic URL), body = `n.Message`, headers `Title`, `Priority` (critical=urgent), `Tags` (warning/rotating_light).
  - `discord`: `POST <url>` (webhook URL) JSON `{"content": n.Message}`.
  - `webhook`: `POST <url>` JSON of `n`. (HMAC signing deferred — see Public API webhook unification.)
  - Stdlib `http.Client{Timeout: 8s}`, best-effort, log failures at Warn. Reuse the client shape from `internal/agent/sender/sender.go`.
- **`engine.go`** — `Run(ctx, Config)` ctx-goroutine ticker, modelled on `internal/hub/retention/retention.go`:
  - tick every `eval_interval` (settings key `alerts.eval_interval`, default 15s, read each tick like retention reads its interval).
  - `snap := store.Snapshot()`; registered hosts via `hosts.List(ctx, db)` (needed for `offline`).
  - in-memory `map[ruleID_host]*state{ pendingSince time.Time; firing bool; eventID int64 }` (engine-local, not persisted — rebuild on restart; firing alerts re-detect within one tick).
  - per enabled rule × target host(s):
    - metric rule: read `snap[host].<metric>`; if host absent → skip (can't evaluate); `breach = compare(value, comparator, threshold)`.
    - `offline` rule: `breach = host absent from snap OR snap[host].Ts older than max(for_seconds, 60s) before now`.
    - transitions: breach && !firing → set/keep `pendingSince`; once `now-pendingSince >= for_seconds` → **FIRING**: insert `alert_events(state=firing)`, `notify(firing)` to all enabled channels, `firing=true`. !breach && firing → **RESOLVED**: set `resolved_at`, `notify(resolved)`, `firing=false`, clear `pendingSince`. !breach && !firing → clear `pendingSince`.
  - Channel routing (Milestone A): dispatch to **all enabled channels**. (Per-rule routing later.)
- **`handlers.go`** — session-protected JSON handlers (mirror `internal/hub/hosts/handlers.go` write helpers + `{"error":...}` shape):
  - `GET/POST /api/alerts/rules`, `PUT/DELETE /api/alerts/rules/{id}`
  - `GET/POST /api/alerts/channels`, `PUT/DELETE /api/alerts/channels/{id}`, `POST /api/alerts/channels/{id}/test` (dispatch a synthetic notification, return ok/error)
  - `GET /api/alerts/events?state=firing|all&limit=N` (active + recent)
- **`engine_test.go`** — table test of the state machine with a fake snapshot map: breach below `for` → no fire; breach past `for` → fire once; clear → resolve. No network.

### Wiring (`internal/hub/server/server.go` + `cmd/lumen-hub/main.go`)
- `server.Config` += `AlertEvalInterval time.Duration`; `cmd/lumen-hub/main.go` env `LUMEN_HUB_ALERT_INTERVAL` (default 15s), pass through (same pattern as `AgentInterval`).
- `settings.EnsureDefaults` seed `alerts.eval_interval` (add `KeyAlertEvalInterval = "alerts.eval_interval"` in `internal/hub/settings/settings.go`).
- In `server.Run`: construct `alertsHandlers := alerts.NewHandlers(db, logger)`; start `go alerts.Run(ctx, alerts.Config{DB: db, Store: st, DefaultInterval: cfg.AlertEvalInterval, Logger: logger.With("subsys","alerts")})` (engine needs both `db` and the existing `st *store.Store`).
- Register the routes inside the existing `r.Group(func(r chi.Router){ r.Use(requireSession); ... })` block.

## Frontend (`web/src`)

- **`lib/api.ts`** — add `alertsApi` using the existing `api<T>()` wrapper: `rules.list/create/update/remove`, `channels.list/create/update/remove/test`, `events(state?)`. Add TS types `AlertRule`, `NotificationChannel`, `AlertEvent`.
- **Top-level tab** — `components/AppShell.tsx`: extend `Tab` to `"dashboard" | "settings" | "alerts"`, add a `<TabButton>` (label `t("shell.alerts")`); `App.tsx`: handle `tab === "alerts"` → render `<Alerts/>`.
- **`components/Alerts.tsx`** — sections, each a `Surface` (reuse `components/ui.tsx`): **Active** (events `state=firing`, danger `StatusPill`, host + message + age), **Recent** (resolved), **Rules** (table + add/edit form: name, metric select, comparator, threshold, for-duration, host (all/specific), severity, enabled), **Channels** (table + add/edit: name, type select, type-specific config fields, + **Test** button). Reuse `Field/FieldInput/PrimaryButton/GhostButton/ErrorText` (`components/CenterCard.tsx`) and the submit/error pattern from `Settings.tsx` `RuntimeSettings`. Poll `events` every ~15s (request-id dedup like `HostDetail.tsx`).
- **i18n** — `i18n/messages.ts`: new top-level `alerts: {}` block in `en` and `vi` (the `WidenStrings` type forces parity); add `shell.alerts`.

## Docs

- `docs/src/content/docs/configure/alerts.md`: rule model (metric/comparator/for), the `offline` rule, channel setup (ntfy topic URL, Discord webhook URL, generic webhook), Test button, and the "alerts debounce via for-duration; eval interval default 15s" note. Add to README feature table + sidebar.

## Verification (end-to-end, Milestone A)

1. `go build ./... && go vet ./... && go test ./...` (incl. `internal/hub/alerts` engine test); `cd web && npx tsc --noEmit`.
2. Docker: build hub image, run on :8090.
3. Alerts tab → Channels → add **ntfy** (`url = https://ntfy.sh/<your-topic>`), subscribe to that topic in the ntfy app/web → **Test** → push arrives.
4. Add a rule `cpu_pct gt 50 for 0s` (all hosts); run an agent and load CPU (`yes > /dev/null`) → within ~15s an alert fires + ntfy push; the Active section shows it; stop the load → it resolves + a resolved push.
5. Discord channel: add a Discord channel-webhook URL → Test → message in the channel.

## Build order

1. Migration `0008_alerts.sql`.
2. `alerts`: `rules.go`, `channels.go` (DB CRUD).
3. `alerts`: `notify.go` + `engine.go` + `engine_test.go`.
4. `alerts`: `handlers.go`; wire into `server.go` + `main.go` + settings key/env.
5. Frontend: `alertsApi`, AppShell tab, `Alerts.tsx`, i18n.
6. Docs `configure/alerts.md` + README/sidebar.
7. Gates + Docker ntfy test. Commit per step; tag a release (v0.4.0) when Milestone A is verified.

## Notes / decisions to honour

- Notification delivery here is **admin-owned**; keep the table forward-compatible with the Public API customer-webhook reuse (owner_type column). Don't build the Public API parts now.
- No new Go deps (stdlib `net/http`/`encoding/json` only).
- Engine state is in-memory (rebuilt on restart) — acceptable; persisted `alert_events` is the source of truth for the UI/history.
- "Feature done = docs done" (ACTION_PLAN rule): `configure/alerts.md` must land with the feature.
