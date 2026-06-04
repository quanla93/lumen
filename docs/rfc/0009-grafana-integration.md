# RFC 0009 — External API export + Grafana spike

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 9
- **Effort**: 8 days

## Motivation

Public Read API (`/api/v1/*`, shipped v0.5) already exposes hosts + metrics + alerts via Bearer-key auth. The unanswered question is **how an operator actually wires Grafana to it**. Beszel ships a Grafana datasource plugin; Lumen has no equivalent. The spike answers:

- Does the existing JSON API work with the open-source `marcusolsson-json-datasource` plugin out of the box?
- Or do we need a Prometheus-compatible subset (`/query`, `/query_range`, `/label/{name}/values`) so the operator can use the native Prometheus datasource?
- What's the recommended dashboard skeleton (sample dashboard JSON checked into the repo)?

The deliverable is BOTH the integration recipe AND any new endpoints required to make it pleasant.

## Scope

**In**:
- Spike: actually configure Grafana against the existing v1 endpoints; identify gaps.
- If JSON-datasource path is viable: write `docs/integrations/grafana.md` with screenshot-led setup + sample dashboard JSON. No new endpoints.
- If Prometheus-compat is needed: implement a minimal subset of `/api/v1/prometheus/api/v1/{query,query_range,label/__name__/values}` mapping to Lumen's metric schema. +3 days within the same sprint.
- Sample dashboard JSON committed to `examples/grafana/` covering Dashboard-equivalent + Host detail-equivalent panels.

**Out**:
- A Lumen-branded Grafana plugin (much bigger; defer).
- Push-to-Grafana (Grafana queries Lumen, not the reverse).
- Annotations from Lumen alerts (nice-to-have; depends on integration path).

## Design

### Spike checklist

1. Provision Grafana (Docker `grafana/grafana:latest`).
2. Add Lumen API key with scopes `read:hosts`, `read:metrics`, `read:alerts`.
3. Install JSON datasource plugin; point it at `/api/v1` with the Bearer key.
4. Try: build a host-CPU time-series panel.
5. Try: build a top-N hosts by RAM table.
6. Try: alert log table from `/api/v1/alerts/events`.
7. Catalog: what worked, what required custom transformations, what was impossible.

### If JSON-datasource path is viable
- Document the steps above into `docs/integrations/grafana.md`.
- Save dashboards as JSON, commit to `examples/grafana/`.
- Done.

### If Prometheus-compat is needed (likely for time-series panels)
- Subset implementation:
  - `/api/v1/prometheus/api/v1/query?query=<metric>{labels}` — instant query, returns the latest scalar.
  - `/api/v1/prometheus/api/v1/query_range?query=<metric>{labels}&start=&end=&step=` — bucketed range. Maps to existing `/api/v1/hosts/{name}/metrics`.
  - `/api/v1/prometheus/api/v1/label/__name__/values` — returns supported metric names.
- PromQL parser: only the subset Lumen actually understands (single-metric + label selectors). Fail with a useful error on advanced syntax.
- Auth: existing Bearer-key middleware.
- Scope: `read:metrics`.

### Sample dashboards

- `examples/grafana/lumen-overview.json` — fleet-wide CPU/RAM/disk, top-N hosts, alert events table.
- `examples/grafana/lumen-host-detail.json` — single-host CPU per-core, RAM, disk I/O, network throughput, recent alerts.

## Risks

| Risk | Mitigation |
|---|---|
| JSON datasource plugin lacks features Grafana operators expect (annotations, alerting) | Document the gaps; offer Prometheus-compat as the recommended path if so. |
| PromQL subset confuses operators who write real PromQL | Document the supported subset prominently; return clear error on unsupported syntax. |
| Maintenance burden of two query styles | JSON datasource is operator-side; Lumen owns Prometheus-compat. Acceptable if we cap surface. |
| Rate limits on Prometheus-style queries (Grafana refreshes every 30s × N panels) | Bump per-key burst; documented. |

## Testing

- Spike's output IS the test. Snapshot Grafana dashboard renders against a seeded Lumen.
- Unit tests for the PromQL subset parser if we ship it.

## Docs deliverables

- `docs/integrations/grafana.md` (datasource setup + sample dashboards).
- If Prometheus path: `docs/integrations/grafana-prometheus.md` covering the differences.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. JSON datasource by default or Prometheus-compat by default? Decide after spike. Proposed: whichever yields a usable time-series + table-of-events dashboard with fewer transformations.
2. Should the sample dashboard import a Lumen logo / theme? Proposed: no — Grafana dashboards should look like Grafana.
3. Annotations from alert events? Proposed: yes if Prometheus path is chosen — add `/api/v1/prometheus/api/v1/query_exemplars` mapping. Defer if JSON path is chosen.
