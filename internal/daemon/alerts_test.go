package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
	"github.com/RakhaYandra/appscope/internal/store"
)

type fakeNotifier struct {
	calls []string
	err   error
}

func (f *fakeNotifier) notify(appName, metric string, current, threshold float64, severity string) error {
	f.calls = append(f.calls, appName+":"+metric+":"+severity)
	return f.err
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "appscope.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestEvaluateFiresAndLogs(t *testing.T) {
	st := testStore(t)
	_, err := st.AddAlertRule("firefox", "cpu_percent", 80)
	if err != nil {
		t.Fatal(err)
	}
	nf := &fakeNotifier{}
	ev := &AlertEvaluator{Store: st, notify: nf.notify, cooldown: 10 * time.Minute}

	violations := ev.Evaluate([]models.AppMetrics{
		{AppName: "firefox", CPUPercent: 90, MemoryRSSBytes: 1000},
	})
	if len(violations) != 1 {
		t.Fatalf("violations = %d, want 1", len(violations))
	}
	if len(nf.calls) != 1 {
		t.Fatalf("notify calls = %d, want 1", len(nf.calls))
	}

	var count int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM threshold_alerts WHERE app_name = 'firefox'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("threshold_alerts rows = %d, want 1", count)
	}
}

func TestEvaluateCriticalSeverityAndCooldown(t *testing.T) {
	st := testStore(t)
	ruleID, err := st.AddAlertRule("firefox", "cpu_percent", 80)
	if err != nil {
		t.Fatal(err)
	}
	nf := &fakeNotifier{}
	ev := &AlertEvaluator{Store: st, notify: nf.notify, cooldown: 10 * time.Minute}

	apps := []models.AppMetrics{{AppName: "firefox", CPUPercent: 130}}
	violations := ev.Evaluate(apps)
	if len(violations) != 1 || violations[0].Severity != "CRITICAL" {
		t.Fatalf("severity = %+v, want CRITICAL", violations)
	}
	_ = ruleID
	nf.calls = nil
	ev.Evaluate(apps)
	if len(nf.calls) != 0 {
		t.Error("cooldown must suppress repeat notification within 10 minutes")
	}
	var count int
	if err := st.DB().QueryRow("SELECT COUNT(*) FROM threshold_alerts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("rows = %d, want 2 (log every violation even when notify suppressed)", count)
	}
}

func TestEvaluateWildcardRule(t *testing.T) {
	st := testStore(t)
	if _, err := st.AddAlertRule("*", "memory_rss_bytes", 1000); err != nil {
		t.Fatal(err)
	}
	nf := &fakeNotifier{}
	ev := &AlertEvaluator{Store: st, notify: nf.notify, cooldown: time.Minute}
	violations := ev.Evaluate([]models.AppMetrics{
		{AppName: "firefox", MemoryRSSBytes: 2000},
		{AppName: "code", MemoryRSSBytes: 100},
	})
	if len(violations) != 1 {
		t.Fatalf("violations = %d, want 1 (only firefox over threshold)", len(violations))
	}
	if violations[0].AppName != "firefox" {
		t.Errorf("app = %s", violations[0].AppName)
	}
}

func TestEvaluateNoViolationNoNotify(t *testing.T) {
	st := testStore(t)
	if _, err := st.AddAlertRule("firefox", "cpu_percent", 80); err != nil {
		t.Fatal(err)
	}
	nf := &fakeNotifier{}
	ev := &AlertEvaluator{Store: st, notify: nf.notify, cooldown: time.Minute}
	violations := ev.Evaluate([]models.AppMetrics{{AppName: "firefox", CPUPercent: 10}})
	if len(violations) != 0 || len(nf.calls) != 0 {
		t.Errorf("violations=%d calls=%d, want 0/0", len(violations), len(nf.calls))
	}
}

func TestCriticalMultiplierBoundary(t *testing.T) {
	st := testStore(t)
	if _, err := st.AddAlertRule("x", "cpu_percent", 100); err != nil {
		t.Fatal(err)
	}
	nf := &fakeNotifier{}
	ev := &AlertEvaluator{Store: st, notify: nf.notify, cooldown: time.Minute}
	cases := []struct {
		cpu          float64
		wantSeverity string
	}{
		{149.9, "WARNING"},
		{150.0, "CRITICAL"},
	}
	for _, tc := range cases {
		v := ev.Evaluate([]models.AppMetrics{{AppName: "x", CPUPercent: tc.cpu}})
		if len(v) != 1 || v[0].Severity != tc.wantSeverity {
			t.Errorf("cpu %f: severity = %+v, want %s", tc.cpu, v, tc.wantSeverity)
		}
	}
}
