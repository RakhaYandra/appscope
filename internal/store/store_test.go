package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "appscope.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesSchemaAndWAL(t *testing.T) {
	s := openTemp(t)
	journal := ""
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}
	for _, table := range []string{"system_metrics", "application_metrics", "threshold_alerts"} {
		var name string
		err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestInsertAndLatestSystemMetric(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC().Truncate(time.Second)
	err := s.InsertSystemMetrics(models.SystemMetrics{
		Timestamp:        now,
		CPUPercent:       42.5,
		MemoryUsedBytes:  9 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 16 * 1024 * 1024 * 1024,
		DiskReadBps:      12000000,
		DiskWriteBps:     4000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.LatestSystemMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if got.CPUPercent != 42.5 {
		t.Errorf("cpu = %f, want 42.5", got.CPUPercent)
	}
	if got.MemoryTotalBytes != 16*1024*1024*1024 {
		t.Errorf("mem total = %d", got.MemoryTotalBytes)
	}
}

func TestInsertAndQueryAppMetricsSince(t *testing.T) {
	s := openTemp(t)
	now := time.Now().UTC()
	for i, app := range []string{"firefox", "code", "firefox"} {
		err := s.InsertAppMetrics([]models.AppMetrics{{
			AppName:        app,
			ProcessCount:   i + 1,
			CPUPercent:     float64(10 * (i + 1)),
			MemoryRSSBytes: uint64(1000 * (i + 1)),
			DiskReadBps:    uint64(100 * (i + 1)),
			DiskWriteBps:   uint64(50 * (i + 1)),
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = now
	rows, err := s.AppMetricsSince("firefox", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.AppName != "firefox" {
			t.Errorf("app = %q, want firefox", r.AppName)
		}
	}
}

func TestOpenInvalidPathFails(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "no-such-dir", "appscope.db")); err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

func TestLatestSystemMetricsEmptyDBFails(t *testing.T) {
	s := openTemp(t)
	if _, err := s.LatestSystemMetrics(); err == nil {
		t.Fatal("expected error on empty database")
	}
}

func TestAppMetricsSinceNoRows(t *testing.T) {
	s := openTemp(t)
	rows, err := s.AppMetricsSince("nonexistent", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestInsertAppMetricsMultipleApps(t *testing.T) {
	s := openTemp(t)
	err := s.InsertAppMetrics([]models.AppMetrics{
		{AppName: "a", ProcessCount: 1, CPUPercent: 1, MemoryRSSBytes: 1},
		{AppName: "b", ProcessCount: 2, CPUPercent: 2, MemoryRSSBytes: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.AppMetricsSince("b", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ProcessCount != 2 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestPruneOnEmptyDatabase(t *testing.T) {
	s := openTemp(t)
	n, err := s.Prune(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("pruned = %d, want 0", n)
	}
}

func TestPruneDeletesOldRows(t *testing.T) {
	s := openTemp(t)
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	fresh := time.Now().UTC().Add(-1 * time.Hour)

	for _, ts := range []time.Time{old, fresh} {
		_, err := s.db.Exec(
			"INSERT INTO system_metrics (timestamp, cpu_percent, memory_used_bytes, memory_total_bytes, disk_read_bps, disk_write_bps) VALUES (?, 1, 1, 1, 1, 1)",
			ts.Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.db.Exec(
			"INSERT INTO application_metrics (timestamp, app_name, process_count, cpu_percent, memory_rss_bytes, disk_read_bytes, disk_write_bytes) VALUES (?, 'x', 1, 1, 1, 1, 1)",
			ts.Format("2006-01-02 15:04:05"))
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.Prune(time.Now().UTC().Add(-7 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("pruned = %d, want 2 (one per table)", n)
	}
	var sysCount, appCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM system_metrics").Scan(&sysCount); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM application_metrics").Scan(&appCount); err != nil {
		t.Fatal(err)
	}
	if sysCount != 1 || appCount != 1 {
		t.Errorf("remaining sys/app = %d/%d, want 1/1", sysCount, appCount)
	}
}
