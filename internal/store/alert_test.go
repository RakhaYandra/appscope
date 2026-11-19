package store

import (
	"testing"
)

func TestAlertRuleCRUD(t *testing.T) {
	s := openTemp(t)
	id1, err := s.AddAlertRule("firefox", "cpu_percent", 80)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.AddAlertRule("*", "memory_rss_bytes", 4e9)
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Error("rule ids must be unique")
	}

	rules, err := s.ListAlertRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	if rules[0].AppName != "firefox" || rules[0].Metric != "cpu_percent" || rules[0].Threshold != 80 {
		t.Errorf("rule[0] = %+v", rules[0])
	}
	if !rules[0].Enabled {
		t.Error("rule must be enabled by default")
	}

	if err := s.RemoveAlertRule(id1); err != nil {
		t.Fatal(err)
	}
	rules, err = s.ListAlertRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != id2 {
		t.Errorf("remaining rules = %+v", rules)
	}
	if err := s.RemoveAlertRule(9999); err == nil {
		t.Error("removing nonexistent rule must error")
	}
}

func TestAddAlertRuleValidation(t *testing.T) {
	s := openTemp(t)
	for _, tc := range []struct {
		app, metric string
		threshold   float64
	}{
		{"firefox", "not_a_metric", 50},
		{"", "cpu_percent", 50},
		{"firefox", "cpu_percent", 0},
		{"firefox", "cpu_percent", -1},
	} {
		if _, err := s.AddAlertRule(tc.app, tc.metric, tc.threshold); err == nil {
			t.Errorf("AddAlertRule(%q, %q, %f) should fail", tc.app, tc.metric, tc.threshold)
		}
	}
}

func TestAllMetricTypesAccepted(t *testing.T) {
	s := openTemp(t)
	for _, m := range []string{"cpu_percent", "memory_rss_bytes", "disk_read_bytes", "disk_write_bytes"} {
		if _, err := s.AddAlertRule("app", m, 1); err != nil {
			t.Errorf("metric %q should be valid: %v", m, err)
		}
	}
}

func TestInsertThresholdAlert(t *testing.T) {
	s := openTemp(t)
	if err := s.InsertThresholdAlert("firefox", "cpu_percent", 85.3, 80, "WARNING"); err != nil {
		t.Fatal(err)
	}
	var app, metric, severity string
	var current, threshold float64
	err := s.db.QueryRow("SELECT app_name, metric_type, current_value, threshold_value, severity FROM threshold_alerts").
		Scan(&app, &metric, &current, &threshold, &severity)
	if err != nil {
		t.Fatal(err)
	}
	if app != "firefox" || metric != "cpu_percent" || severity != "WARNING" {
		t.Errorf("row = %s %s %s", app, metric, severity)
	}
	if current != 85.3 || threshold != 80 {
		t.Errorf("current/threshold = %f/%f", current, threshold)
	}
}
