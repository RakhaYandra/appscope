package store

import (
	"fmt"
	"time"
)

var validMetrics = map[string]bool{
	"cpu_percent":      true,
	"memory_rss_bytes": true,
	"disk_read_bytes":  true,
	"disk_write_bytes": true,
}

func ValidMetric(m string) bool {
	return validMetrics[m]
}

type AlertRule struct {
	ID        int64
	AppName   string
	Metric    string
	Threshold float64
	Enabled   bool
	CreatedAt time.Time
}

func (s *Store) AddAlertRule(appName, metric string, threshold float64) (int64, error) {
	if appName == "" {
		return 0, fmt.Errorf("app_name must not be empty (use '*' for all apps)")
	}
	if !validMetrics[metric] {
		return 0, fmt.Errorf("invalid metric %q (want cpu_percent|memory_rss_bytes|disk_read_bytes|disk_write_bytes)", metric)
	}
	if threshold <= 0 {
		return 0, fmt.Errorf("threshold must be > 0")
	}
	res, err := s.db.Exec(
		"INSERT INTO alert_rules (app_name, metric, threshold) VALUES (?, ?, ?)",
		appName, metric, threshold)
	if err != nil {
		return 0, fmt.Errorf("insert alert_rule: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) ListAlertRules() ([]AlertRule, error) {
	rows, err := s.db.Query(
		"SELECT id, app_name, metric, threshold, enabled, created_at FROM alert_rules ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("query alert_rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AlertRule
	for rows.Next() {
		var r AlertRule
		var enabled int
		var created string
		if err := rows.Scan(&r.ID, &r.AppName, &r.Metric, &r.Threshold, &enabled, &created); err != nil {
			return nil, fmt.Errorf("scan alert_rule: %w", err)
		}
		r.Enabled = enabled != 0
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RemoveAlertRule(id int64) error {
	res, err := s.db.Exec("DELETE FROM alert_rules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete alert_rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete alert_rule rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("alert rule %d not found", id)
	}
	return nil
}

func (s *Store) InsertThresholdAlert(appName, metric string, current, threshold float64, severity string) error {
	_, err := s.db.Exec(
		"INSERT INTO threshold_alerts (app_name, metric_type, current_value, threshold_value, severity) VALUES (?, ?, ?, ?, ?)",
		appName, metric, current, threshold, severity)
	if err != nil {
		return fmt.Errorf("insert threshold_alert: %w", err)
	}
	return nil
}
