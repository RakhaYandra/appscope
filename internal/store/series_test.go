package store

import (
	"testing"
	"time"
)

func insertSystemAt(t *testing.T, s *Store, ts time.Time, cpu float64, memUsed uint64) {
	t.Helper()
	_, err := s.db.Exec(
		"INSERT INTO system_metrics (timestamp, cpu_percent, memory_used_bytes, memory_total_bytes, disk_read_bps, disk_write_bps) VALUES (?, ?, ?, 16000000000, 1000, 2000)",
		ts.UTC().Format("2006-01-02 15:04:05"), cpu, memUsed)
	if err != nil {
		t.Fatal(err)
	}
}

func insertAppAt(t *testing.T, s *Store, ts time.Time, app string, cpu float64, rss uint64) {
	t.Helper()
	_, err := s.db.Exec(
		"INSERT INTO application_metrics (timestamp, app_name, process_count, cpu_percent, memory_rss_bytes, disk_read_bytes, disk_write_bytes) VALUES (?, ?, 1, ?, ?, 100, 50)",
		ts.UTC().Format("2006-01-02 15:04:05"), app, cpu, rss)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystemMetricsSeriesBuckets(t *testing.T) {
	s := openTemp(t)
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Minute)
	insertSystemAt(t, s, base, 10, 1000)
	insertSystemAt(t, s, base.Add(30*time.Second), 20, 3000)
	insertSystemAt(t, s, base.Add(2*time.Minute), 50, 5000)

	rows, err := s.SystemMetricsSeries(time.Now().Add(-2*time.Hour), 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("buckets = %d, want 2 (%+v)", len(rows), rows)
	}
	if rows[0].AvgCPU != 15 {
		t.Errorf("bucket0 avg cpu = %f, want 15", rows[0].AvgCPU)
	}
	if rows[0].MaxCPU != 20 {
		t.Errorf("bucket0 max cpu = %f, want 20", rows[0].MaxCPU)
	}
	if rows[0].AvgMemoryUsed != 2000 {
		t.Errorf("bucket0 avg mem = %d, want 2000", rows[0].AvgMemoryUsed)
	}
	if rows[0].MaxMemoryUsed != 3000 {
		t.Errorf("bucket0 max mem = %d, want 3000", rows[0].MaxMemoryUsed)
	}
	if rows[1].AvgCPU != 50 || rows[1].MaxCPU != 50 {
		t.Errorf("bucket1 cpu = %f/%f, want 50/50", rows[1].AvgCPU, rows[1].MaxCPU)
	}
	if rows[0].AvgDiskReadBps != 1000 || rows[1].AvgDiskWriteBps != 2000 {
		t.Errorf("disk bps = %d/%d", rows[0].AvgDiskReadBps, rows[1].AvgDiskWriteBps)
	}
}

func TestAppMetricsSeriesBuckets(t *testing.T) {
	s := openTemp(t)
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Minute)
	insertAppAt(t, s, base, "firefox", 10, 1000)
	insertAppAt(t, s, base.Add(30*time.Second), "firefox", 30, 3000)
	insertAppAt(t, s, base.Add(30*time.Second), "code", 99, 9999)

	rows, err := s.AppMetricsSeries("firefox", time.Now().Add(-2*time.Hour), 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("buckets = %d, want 1 (%+v)", len(rows), rows)
	}
	if rows[0].AvgCPU != 20 {
		t.Errorf("avg cpu = %f, want 20", rows[0].AvgCPU)
	}
	if rows[0].MaxCPU != 30 {
		t.Errorf("max cpu = %f, want 30", rows[0].MaxCPU)
	}
	if rows[0].AvgRSS != 2000 {
		t.Errorf("avg rss = %d, want 2000", rows[0].AvgRSS)
	}
	if rows[0].MaxRSS != 3000 {
		t.Errorf("max rss = %d, want 3000", rows[0].MaxRSS)
	}
	if rows[0].Samples != 2 {
		t.Errorf("samples = %d, want 2", rows[0].Samples)
	}
}

func TestAppMetricsAggregateRanksByAvgCPU(t *testing.T) {
	s := openTemp(t)
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Minute)
	for i := 0; i < 3; i++ {
		insertAppAt(t, s, base.Add(time.Duration(i)*time.Minute), "firefox", 20, 1000)
		insertAppAt(t, s, base.Add(time.Duration(i)*time.Minute), "code", 10, 5000)
	}

	rows, err := s.AppMetricsAggregate(time.Now().Add(-2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("apps = %d, want 2", len(rows))
	}
	if rows[0].AppName != "firefox" {
		t.Errorf("first = %s, want firefox (highest avg cpu)", rows[0].AppName)
	}
	if rows[0].AvgCPU != 20 || rows[0].MaxCPU != 20 {
		t.Errorf("firefox cpu = %f/%f", rows[0].AvgCPU, rows[0].MaxCPU)
	}
	if rows[0].Samples != 3 {
		t.Errorf("samples = %d, want 3", rows[0].Samples)
	}
	if rows[1].AvgRSS != 5000 {
		t.Errorf("code avg rss = %d, want 5000", rows[1].AvgRSS)
	}
}

func TestSeriesEmptyRange(t *testing.T) {
	s := openTemp(t)
	rows, err := s.SystemMetricsSeries(time.Now().Add(-time.Hour), 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}
