package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/RakhaYandra/appscope/internal/models"
)

const schema = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS system_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	cpu_percent REAL NOT NULL,
	memory_used_bytes INTEGER NOT NULL,
	memory_total_bytes INTEGER NOT NULL,
	disk_read_bps INTEGER NOT NULL,
	disk_write_bps INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS application_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	app_name TEXT NOT NULL,
	process_count INTEGER NOT NULL,
	cpu_percent REAL NOT NULL,
	memory_rss_bytes INTEGER NOT NULL,
	disk_read_bytes INTEGER NOT NULL,
	disk_write_bytes INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS threshold_alerts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	app_name TEXT NOT NULL,
	metric_type TEXT NOT NULL,
	current_value REAL NOT NULL,
	threshold_value REAL NOT NULL,
	severity TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sys_time ON system_metrics(timestamp);
CREATE INDEX IF NOT EXISTS idx_app_time ON application_metrics(app_name, timestamp);
CREATE TABLE IF NOT EXISTS alert_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	app_name TEXT NOT NULL,
	metric TEXT NOT NULL,
	threshold REAL NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) InsertSystemMetrics(m models.SystemMetrics) error {
	_, err := s.db.Exec(
		"INSERT INTO system_metrics (timestamp, cpu_percent, memory_used_bytes, memory_total_bytes, disk_read_bps, disk_write_bps) VALUES (?, ?, ?, ?, ?, ?)",
		m.Timestamp.UTC().Format("2006-01-02 15:04:05"),
		m.CPUPercent, m.MemoryUsedBytes, m.MemoryTotalBytes, m.DiskReadBps, m.DiskWriteBps)
	if err != nil {
		return fmt.Errorf("insert system_metrics: %w", err)
	}
	return nil
}

func (s *Store) InsertAppMetrics(apps []models.AppMetrics) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(
		"INSERT INTO application_metrics (timestamp, app_name, process_count, cpu_percent, memory_rss_bytes, disk_read_bytes, disk_write_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, a := range apps {
		if _, err := stmt.Exec(now, a.AppName, a.ProcessCount, a.CPUPercent, a.MemoryRSSBytes, a.DiskReadBps, a.DiskWriteBps); err != nil {
			return fmt.Errorf("insert application_metrics %q: %w", a.AppName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

type AppMetricsRow struct {
	Timestamp      time.Time
	AppName        string
	ProcessCount   int
	CPUPercent     float64
	MemoryRSSBytes uint64
	DiskReadBps    uint64
	DiskWriteBps   uint64
}

func (s *Store) AppMetricsSince(app string, since time.Time) ([]AppMetricsRow, error) {
	rows, err := s.db.Query(
		"SELECT timestamp, app_name, process_count, cpu_percent, memory_rss_bytes, disk_read_bytes, disk_write_bytes FROM application_metrics WHERE app_name = ? AND timestamp >= ? ORDER BY timestamp",
		app, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("query application_metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppMetricsRow
	for rows.Next() {
		var r AppMetricsRow
		var ts string
		if err := rows.Scan(&ts, &r.AppName, &r.ProcessCount, &r.CPUPercent, &r.MemoryRSSBytes, &r.DiskReadBps, &r.DiskWriteBps); err != nil {
			return nil, fmt.Errorf("scan application_metrics: %w", err)
		}
		r.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LatestSystemMetrics() (*models.SystemMetrics, error) {
	var m models.SystemMetrics
	var ts string
	err := s.db.QueryRow(
		"SELECT timestamp, cpu_percent, memory_used_bytes, memory_total_bytes, disk_read_bps, disk_write_bps FROM system_metrics ORDER BY id DESC LIMIT 1").
		Scan(&ts, &m.CPUPercent, &m.MemoryUsedBytes, &m.MemoryTotalBytes, &m.DiskReadBps, &m.DiskWriteBps)
	if err != nil {
		return nil, fmt.Errorf("latest system_metrics: %w", err)
	}
	m.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
	return &m, nil
}

func (s *Store) Prune(olderThan time.Time) (int64, error) {
	cutoff := olderThan.UTC().Format("2006-01-02 15:04:05")
	var total int64
	for _, table := range []string{"system_metrics", "application_metrics", "threshold_alerts"} {
		res, err := s.db.Exec("DELETE FROM "+table+" WHERE timestamp < ?", cutoff)
		if err != nil {
			return total, fmt.Errorf("prune %s: %w", table, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("prune %s rows affected: %w", table, err)
		}
		total += n
	}
	return total, nil
}
