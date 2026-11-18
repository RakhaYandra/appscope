package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/appscope/internal/store"
)

func seedReportDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "appscope.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	for i := 0; i < 4; i++ {
		_, err := st.DB().Exec(
			"INSERT INTO system_metrics (timestamp, cpu_percent, memory_used_bytes, memory_total_bytes, disk_read_bps, disk_write_bps) VALUES (?, ?, ?, 16000000000, ?, ?)",
			base.Add(time.Duration(i)*time.Minute).Format("2006-01-02 15:04:05"),
			float64(10*(i+1)), uint64(1000*(i+1)), uint64(100*(i+1)), uint64(50*(i+1)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = st.DB().Exec(
			"INSERT INTO application_metrics (timestamp, app_name, process_count, cpu_percent, memory_rss_bytes, disk_read_bytes, disk_write_bytes) VALUES (?, ?, ?, ?, ?, 100, 50)",
			base.Add(time.Duration(i)*time.Minute).Format("2006-01-02 15:04:05"),
			map[int]string{0: "firefox", 1: "firefox", 2: "code", 3: "firefox"}[i], 1,
			map[int]float64{0: 30, 1: 40, 2: 10, 3: 50}[i], uint64(1000*(i+1)))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func TestBuildReport(t *testing.T) {
	dbPath := seedReportDB(t)

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	rep, err := buildReport(st, time.Now().Add(-2*time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rep.System.MaxCPU != 40 {
		t.Errorf("max cpu = %f, want 40", rep.System.MaxCPU)
	}
	if rep.System.AvgCPU != 25 {
		t.Errorf("avg cpu = %f, want 25", rep.System.AvgCPU)
	}
	if rep.System.MaxMemoryUsedBytes != 4000 {
		t.Errorf("max mem = %d, want 4000", rep.System.MaxMemoryUsedBytes)
	}
	if len(rep.Applications) != 2 {
		t.Fatalf("apps = %d, want 2", len(rep.Applications))
	}
	if rep.Applications[0].AppName != "firefox" {
		t.Errorf("first app = %s, want firefox", rep.Applications[0].AppName)
	}
	if rep.Applications[0].Samples != 3 {
		t.Errorf("firefox samples = %d, want 3", rep.Applications[0].Samples)
	}
	if rep.Applications[0].AvgCPUPercent != 40 {
		t.Errorf("firefox avg cpu = %f, want 40", rep.Applications[0].AvgCPUPercent)
	}
}

func TestBuildReportEmptyDB(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "appscope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rep, err := buildReport(st, time.Now().Add(-time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rep.System.AvgCPU != 0 || len(rep.Applications) != 0 {
		t.Errorf("empty db should yield zero system + no apps, got %+v", rep)
	}
}

func TestRenderReportJSON(t *testing.T) {
	dbPath := seedReportDB(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rep, err := buildReport(st, time.Now().Add(-2*time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := renderReportJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(buf.String()), &parsed); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"generated_at", "duration", "system", "applications"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("json missing key %q", key)
		}
	}
}

func TestRenderReportCSV(t *testing.T) {
	dbPath := seedReportDB(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rep, err := buildReport(st, time.Now().Add(-2*time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := renderReportCSV(&buf, rep); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("csv lines = %d, want 7 ([system] hdr+row, [applications] hdr+2 rows):\n%s", len(lines), buf.String())
	}
	if lines[0] != "[system]" || lines[3] != "[applications]" {
		t.Errorf("unexpected section layout:\n%s", buf.String())
	}
	if !strings.HasPrefix(lines[1], "generated_at,") || !strings.HasPrefix(lines[4], "app_name,") {
		t.Errorf("unexpected csv headers:\n%s", buf.String())
	}
}
