package daemon

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
	"github.com/RakhaYandra/appscope/internal/store"
)

type alertViolation struct {
	RuleID    int64
	AppName   string
	Metric    string
	Current   float64
	Threshold float64
	Severity  string
}

type notifierFunc func(appName, metric string, current, threshold float64, severity string) error

type AlertEvaluator struct {
	Store    *store.Store
	notify   notifierFunc
	cooldown time.Duration

	lastNotified map[int64]time.Time
	now          func() time.Time
}

func newAlertEvaluator(st *store.Store) *AlertEvaluator {
	return &AlertEvaluator{
		Store:        st,
		notify:       NotifySend,
		cooldown:     10 * time.Minute,
		lastNotified: map[int64]time.Time{},
		now:          time.Now,
	}
}

func (e *AlertEvaluator) Evaluate(apps []models.AppMetrics) []alertViolation {
	if e.now == nil {
		e.now = time.Now
	}
	if e.lastNotified == nil {
		e.lastNotified = map[int64]time.Time{}
	}
	rules, err := e.Store.ListAlertRules()
	if err != nil {
		log.Printf("alerts: list rules: %v", err)
		return nil
	}
	var violations []alertViolation
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, app := range apps {
			if rule.AppName != "*" && rule.AppName != app.AppName {
				continue
			}
			current, ok := metricValue(app, rule.Metric)
			if !ok {
				continue
			}
			if current < rule.Threshold {
				continue
			}
			severity := "WARNING"
			if current >= rule.Threshold*1.5 {
				severity = "CRITICAL"
			}
			v := alertViolation{
				RuleID:    rule.ID,
				AppName:   app.AppName,
				Metric:    rule.Metric,
				Current:   current,
				Threshold: rule.Threshold,
				Severity:  severity,
			}
			violations = append(violations, v)
			if err := e.Store.InsertThresholdAlert(v.AppName, v.Metric, v.Current, v.Threshold, v.Severity); err != nil {
				log.Printf("alerts: log threshold alert: %v", err)
			}
			if last, ok := e.lastNotified[rule.ID]; ok && e.now().Sub(last) < e.cooldown {
				continue
			}
			if e.notify != nil {
				if err := e.notify(v.AppName, v.Metric, v.Current, v.Threshold, v.Severity); err != nil {
					log.Printf("alerts: notify-send failed: %v", err)
					continue
				}
			}
			e.lastNotified[rule.ID] = e.now()
		}
	}
	return violations
}

func metricValue(app models.AppMetrics, metric string) (float64, bool) {
	switch metric {
	case "cpu_percent":
		return app.CPUPercent, true
	case "memory_rss_bytes":
		return float64(app.MemoryRSSBytes), true
	case "disk_read_bytes":
		return float64(app.DiskReadBps), true
	case "disk_write_bytes":
		return float64(app.DiskWriteBps), true
	default:
		return 0, false
	}
}

func NotifySend(appName, metric string, current, threshold float64, severity string) error {
	urgency := "normal"
	if severity == "CRITICAL" {
		urgency = "critical"
	}
	title := fmt.Sprintf("AppScope: %s %s", appName, severity)
	body := fmt.Sprintf("%s = %.1f, threshold = %.1f", metric, current, threshold)
	cmd := exec.Command("notify-send", "-u", urgency, title, body)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notify-send: %w", err)
	}
	return nil
}
