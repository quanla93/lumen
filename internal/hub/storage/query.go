package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MetricPoint is one bucket in a downsampled history series. All metric
// fields are AVG-aggregated across the rows that fell in the bucket.
type MetricPoint struct {
	Ts       time.Time `json:"ts"`
	CpuPct   float64   `json:"cpu_pct"`
	RamPct   float64   `json:"ram_pct"`
	SwapPct  float64   `json:"swap_pct"`
	DiskPct  float64   `json:"disk_pct"`
	Load1    float64   `json:"load1"`
	Load5    float64   `json:"load5"`
	Load15   float64   `json:"load15"`
	NetRxBps float64   `json:"net_rx_bps"`
	NetTxBps float64   `json:"net_tx_bps"`
	DiskRBps float64   `json:"disk_r_bps"`
	DiskWBps float64   `json:"disk_w_bps"`
	TempC    float64   `json:"temp_c"`
}

// QueryMetrics returns downsampled samples for host between [from, to).
// Each bucket spans stepSeconds and is the AVG of rows whose ts falls in
// it. Empty buckets are NOT padded — callers fill gaps client-side if a
// continuous series is required. Results are ordered by bucket start asc.
//
// The (host, ts) index from migration 0001 covers the WHERE+GROUP path.
func QueryMetrics(
	ctx context.Context, db *sql.DB,
	host string, from, to time.Time, stepSeconds int64,
) ([]MetricPoint, error) {
	if stepSeconds <= 0 {
		return nil, fmt.Errorf("step must be > 0")
	}
	if !from.Before(to) {
		return nil, fmt.Errorf("from must be < to")
	}
	rows, err := db.QueryContext(ctx, `
			WITH parsed AS (
				SELECT
					CAST(strftime('%s', ts) AS INTEGER) AS unix_ts,
					cpu_pct, ram_pct, swap_pct, disk_pct,
					load1, load5, load15,
					net_rx_bps, net_tx_bps,
					disk_r_bps, disk_w_bps,
					temp_c
				FROM snapshots
				WHERE host = ? AND ts >= ? AND ts < ?
			)
			SELECT
				unix_ts / ? * ? AS bucket,
				AVG(cpu_pct), AVG(ram_pct), AVG(swap_pct), AVG(disk_pct),
				AVG(load1), AVG(load5), AVG(load15),
				AVG(net_rx_bps), AVG(net_tx_bps),
				AVG(disk_r_bps), AVG(disk_w_bps),
				AVG(temp_c)
			FROM parsed
			WHERE unix_ts IS NOT NULL
			GROUP BY bucket
			ORDER BY bucket ASC
		`, host, formatTS(from), formatTS(to), stepSeconds, stepSeconds)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var out []MetricPoint
	for rows.Next() {
		var bucket int64
		var p MetricPoint
		if err := rows.Scan(
			&bucket,
			&p.CpuPct, &p.RamPct, &p.SwapPct, &p.DiskPct,
			&p.Load1, &p.Load5, &p.Load15,
			&p.NetRxBps, &p.NetTxBps,
			&p.DiskRBps, &p.DiskWBps,
			&p.TempC,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		p.Ts = time.Unix(bucket, 0).UTC()
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSnapshotsBefore drops every row with ts < cutoff. Returns the
// number of rows deleted so the caller can log retention activity.
func DeleteSnapshotsBefore(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM snapshots WHERE ts < ?`, formatTS(cutoff))
	if err != nil {
		return 0, fmt.Errorf("delete snapshots: %w", err)
	}
	return res.RowsAffected()
}

// DeleteResolvedAlertsBefore prunes alert_events older than cutoff that
// are no longer firing. We never delete a still-firing row — operators
// should always see active breaches in the History tab regardless of age.
//
// The notification_deliveries rows attached to a deleted event are reaped
// by the ON DELETE CASCADE from migration 0011, so deliveries for old
// resolved events come away on the same sweep without needing a join here.
//
// COALESCE(resolved_at, started_at) gives the row's "newest meaningful
// timestamp": for resolved rows that's the resolution time; for ghost
// rows that lost their resolved_at to a crash we fall back to started_at
// so they still age out. datetime() normalises across the two formats
// SQLite holds — CURRENT_TIMESTAMP writes 'YYYY-MM-DD HH:MM:SS', whereas
// engine.markResolved bound a Go time.Time which the driver renders as
// RFC3339Nano.
func DeleteResolvedAlertsBefore(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM alert_events
		WHERE state = 'resolved'
		  AND datetime(COALESCE(resolved_at, started_at)) < datetime(?)`,
		formatTS(cutoff),
	)
	if err != nil {
		return 0, fmt.Errorf("delete alert events: %w", err)
	}
	return res.RowsAffected()
}

// DeleteTerminalDeliveriesBefore prunes notification_deliveries in a
// terminal state (sent/failed/dropped) older than cutoff. Pending and
// inflight rows are never touched — the dispatcher is still working on
// them. This runs IN ADDITION to the ON DELETE CASCADE from the alerts
// sweep so deliveries for still-firing events (a chronic problem with
// weeks of `sent` rows) also get pruned.
func DeleteTerminalDeliveriesBefore(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM notification_deliveries
		WHERE status IN ('sent', 'failed', 'dropped')
		  AND datetime(COALESCE(sent_at, created_at)) < datetime(?)`,
		formatTS(cutoff),
	)
	if err != nil {
		return 0, fmt.Errorf("delete notification deliveries: %w", err)
	}
	return res.RowsAffected()
}
