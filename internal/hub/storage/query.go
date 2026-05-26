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
	Ts      time.Time `json:"ts"`
	CpuPct  float64   `json:"cpu_pct"`
	RamPct  float64   `json:"ram_pct"`
	SwapPct float64   `json:"swap_pct"`
	DiskPct float64   `json:"disk_pct"`
	Load1   float64   `json:"load1"`
	Load5   float64   `json:"load5"`
	Load15  float64   `json:"load15"`
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
		SELECT
			CAST(strftime('%s', ts) AS INTEGER) / ? * ? AS bucket,
			AVG(cpu_pct), AVG(ram_pct), AVG(swap_pct), AVG(disk_pct),
			AVG(load1), AVG(load5), AVG(load15)
		FROM snapshots
		WHERE host = ? AND ts >= ? AND ts < ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`, stepSeconds, stepSeconds, host, from.UTC(), to.UTC())
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
	res, err := db.ExecContext(ctx, `DELETE FROM snapshots WHERE ts < ?`, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("delete snapshots: %w", err)
	}
	return res.RowsAffected()
}
