# ADR-0001: Storage architecture — SQLite (hot) + Parquet (cold)

- **Status**: Accepted
- **Date**: 2026-05-25
- **Deciders**: Core team
- **Supersedes**: —
- **Superseded by**: —

## Context

Lumen needs to store time-series metrics from 1 to 200 hosts, each emitting ~10-20 numeric metrics every 5 seconds. We need to:

1. Serve realtime dashboards (sub-second freshness).
2. Answer queries from "last hour" to "last 30 days" cheaply.
3. **Minimize disk writes** — many homelab users run Lumen on HDDs or low-endurance SSDs.
4. Keep deployment to a single binary (no external database).
5. Keep RAM footprint < 100 MB even with 200 hosts active.

The candidates we considered:

| Option | Embed? | Time-series fit | HDD-friendly | RAM | Complexity |
|---|---|---|---|---|---|
| **Prometheus TSDB** | Library exists but heavy | Excellent | ⚠️ High write amp | 200-500 MB | High |
| **InfluxDB / IOx** | No (server) | Excellent | ⚠️ | 300 MB+ | Medium |
| **TimescaleDB** | No (Postgres ext) | Excellent | ⚠️ | 150 MB+ | High |
| **MongoDB time-series** | No (server) | Good | ❌ Write amp | 200 MB+ | High |
| **DuckDB** | Yes | Excellent (OLAP) | ✅ Read-heavy | 30 MB+ | Medium |
| **SQLite** | Yes | OK with indexing | ✅ Best with WAL | Minimal | Low |
| **Parquet files** | Yes (lib) | Excellent (columnar) | ✅ Cold tier ideal | Minimal | Low |

We considered combining options. The key insight: **realtime + recent** workloads differ from **historical** workloads. So pick the right tool for each.

## Decision

Three-tier storage:

1. **Tier 0 — RAM ring buffer**: per-host circular buffer holding ~15 minutes of raw 5-second samples. Serves the live dashboard and the first paint of any host detail page. Never hits disk.

2. **Tier 1 — SQLite (hot)**: 1-minute bucketed metrics for the last 24 hours. Schema is a *narrow table* (one column per core metric), not key-value, because the core metric set is fixed and small. WAL mode, `synchronous=NORMAL`, batch INSERTs every ~60 seconds.

3. **Tier 2 — Parquet (cold)**: 5-minute bucketed metrics for retention up to 365 days. ZSTD compressed. Written hourly by a compaction job that reads from SQLite, downsamples, appends to a Parquet file per (host, day), then deletes the migrated SQLite rows.

For optional analytical queries over the cold tier (range >30 days, ad-hoc aggregations), we may **embed DuckDB read-only** in a later version. SQLite is the only writer at hot tier.

## Consequences

### Positive

- **Tiny binary footprint**: SQLite (~1 MB) and Parquet (~3 MB) libraries vs Prometheus TSDB (heavier).
- **Low write amplification**:
  - Batch INSERTs every 60s mean ~1440 transactions/day on SQLite.
  - Parquet compaction once/hour writes a single contiguous file per day per host.
- **Excellent realtime path**: dashboard reads come from RAM, never touch disk.
- **Single-file backups**: copy `lumen.db` + `parquet/` directory.
- **Cold-tier queries are fast**: Parquet columnar layout + ZSTD reads quickly.
- **No external service**: still single-binary deploy.

### Negative

- **No PromQL**: queries are written in our own narrow API. We have to design and document it. Mitigation: keep queries simple — homelab users don't need PromQL.
- **Compaction job is a moving part**: must be idempotent and recoverable. Mitigation: write Parquet to a temp file, fsync, rename, then delete SQLite rows in a transaction.
- **Adding new "tag dimensions"** (Prometheus-style labels) is awkward in a narrow schema. Mitigation: add a separate `metrics_custom(host_id, key, ts, value)` table for tagged extensions. Trade-off explicit: tag explosion is not supported, by design.
- **Schema migrations** must be careful — SQLite is fine with `ALTER TABLE ADD COLUMN` but indexes need planning.
- **Parquet append is not strictly append-only**: we write a new file per (host, day) and merge at end of day. Manageable.

### Neutral

- We can swap Parquet for another columnar format later if needed — the cold-tier interface is internal.
- We may add DuckDB later for cold-tier read queries without changing the writer.

## Alternatives rejected

- **Prometheus TSDB library** (`tsdb` package): designed for high-cardinality multi-target, write amplification too high for our HDD-friendly goal, RAM overhead high.
- **MongoDB**: violates single-binary goal, write amp on HDDs known to be poor.
- **InfluxDB**: violates single-binary goal, RAM heavy.
- **DuckDB as primary store**: excellent for analytical reads but not designed for high-frequency small writes. Considered for cold tier read engine only.
- **VictoriaMetrics**: excellent but is a separate process and ecosystem; pulls us toward Prometheus-shaped UX which we explicitly don't want.

## References

- Beszel — uses PocketBase (SQLite) only, no cold tier. Inspiration for "SQLite is enough for homelab."
- DuckDB benchmarks on Parquet — informed the cold-tier choice.
- SQLite WAL + `synchronous=NORMAL` guidance: https://www.sqlite.org/wal.html
- Internal benchmark target: < 50 MB disk writes/day per host averaged.

## Related ADRs

- [[ADR-0002: Transport choice (HTTPS/WS over SSH)]]
- [[ADR-0003: Language choice (Go for hub + agent)]]
