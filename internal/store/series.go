package store

import (
	"fmt"
	"time"
)

const tsLayout = "2006-01-02 15:04:05"

type SystemSeriesRow struct {
	BucketStart     time.Time
	AvgCPU          float64
	MaxCPU          float64
	AvgMemoryUsed   uint64
	MaxMemoryUsed   uint64
	AvgDiskReadBps  uint64
	AvgDiskWriteBps uint64
}

func (s *Store) SystemMetricsSeries(since time.Time, bucketSeconds int) ([]SystemSeriesRow, error) {
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucketSeconds must be > 0")
	}
	rows, err := s.db.Query(
		`SELECT CAST(strftime('%s', timestamp) AS INTEGER) / ? AS bucket,
			AVG(cpu_percent), MAX(cpu_percent),
			CAST(AVG(memory_used_bytes) AS INTEGER), CAST(MAX(memory_used_bytes) AS INTEGER),
			CAST(AVG(disk_read_bps) AS INTEGER), CAST(AVG(disk_write_bps) AS INTEGER)
		FROM system_metrics
		WHERE timestamp >= ?
		GROUP BY bucket
		ORDER BY bucket`,
		bucketSeconds, since.UTC().Format(tsLayout))
	if err != nil {
		return nil, fmt.Errorf("query system series: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SystemSeriesRow
	for rows.Next() {
		var bucket int64
		var r SystemSeriesRow
		if err := rows.Scan(&bucket, &r.AvgCPU, &r.MaxCPU, &r.AvgMemoryUsed, &r.MaxMemoryUsed, &r.AvgDiskReadBps, &r.AvgDiskWriteBps); err != nil {
			return nil, fmt.Errorf("scan system series: %w", err)
		}
		r.BucketStart = time.Unix(bucket*int64(bucketSeconds), 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

type AppSeriesRow struct {
	BucketStart time.Time
	AvgCPU      float64
	MaxCPU      float64
	AvgRSS      uint64
	MaxRSS      uint64
	AvgDiskBps  uint64
	MaxDiskBps  uint64
	Samples     int
}

func (s *Store) AppMetricsSeries(app string, since time.Time, bucketSeconds int) ([]AppSeriesRow, error) {
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucketSeconds must be > 0")
	}
	rows, err := s.db.Query(
		`SELECT CAST(strftime('%s', timestamp) AS INTEGER) / ? AS bucket,
			AVG(cpu_percent), MAX(cpu_percent),
			CAST(AVG(memory_rss_bytes) AS INTEGER), CAST(MAX(memory_rss_bytes) AS INTEGER),
			CAST(AVG(disk_read_bytes + disk_write_bytes) AS INTEGER), CAST(MAX(disk_read_bytes + disk_write_bytes) AS INTEGER),
			COUNT(*)
		FROM application_metrics
		WHERE app_name = ? AND timestamp >= ?
		GROUP BY bucket
		ORDER BY bucket`,
		bucketSeconds, app, since.UTC().Format(tsLayout))
	if err != nil {
		return nil, fmt.Errorf("query app series: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppSeriesRow
	for rows.Next() {
		var bucket int64
		var r AppSeriesRow
		if err := rows.Scan(&bucket, &r.AvgCPU, &r.MaxCPU, &r.AvgRSS, &r.MaxRSS, &r.AvgDiskBps, &r.MaxDiskBps, &r.Samples); err != nil {
			return nil, fmt.Errorf("scan app series: %w", err)
		}
		r.BucketStart = time.Unix(bucket*int64(bucketSeconds), 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

type AppAggregateRow struct {
	AppName string
	AvgCPU  float64
	MaxCPU  float64
	AvgRSS  uint64
	MaxRSS  uint64
	Samples int
}

func (s *Store) AppMetricsAggregate(since time.Time) ([]AppAggregateRow, error) {
	rows, err := s.db.Query(
		`SELECT app_name,
			AVG(cpu_percent), MAX(cpu_percent),
			CAST(AVG(memory_rss_bytes) AS INTEGER), CAST(MAX(memory_rss_bytes) AS INTEGER),
			COUNT(*)
		FROM application_metrics
		WHERE timestamp >= ?
		GROUP BY app_name
		ORDER BY AVG(cpu_percent) DESC`,
		since.UTC().Format(tsLayout))
	if err != nil {
		return nil, fmt.Errorf("query app aggregate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppAggregateRow
	for rows.Next() {
		var r AppAggregateRow
		if err := rows.Scan(&r.AppName, &r.AvgCPU, &r.MaxCPU, &r.AvgRSS, &r.MaxRSS, &r.Samples); err != nil {
			return nil, fmt.Errorf("scan app aggregate: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
