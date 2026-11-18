package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintOverviewRendersSections(t *testing.T) {
	v := view{
		Source: "test",
		System: systemView{CPUPercent: 42.5, MemoryUsedGB: 9, MemoryTotalGB: 16, DiskReadMBps: 12, DiskWriteMBps: 4, ActiveProcesses: 100},
		Apps: []appView{
			{Name: "firefox", ProcessCount: 5, CPUPercent: 18.2, MemoryRSSBytes: 2 * 1024 * 1024 * 1024},
		},
	}
	var buf bytes.Buffer
	printOverview(&buf, v)
	out := buf.String()
	for _, want := range []string{"AppScope", "42.5", "firefox", "PROCS"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing %q\n%s", want, out)
		}
	}
}

func TestSortAndFilterApps(t *testing.T) {
	apps := []appView{
		{Name: "code", CPUPercent: 8, MemoryRSSBytes: 100, DiskReadBps: 50},
		{Name: "firefox", CPUPercent: 18, MemoryRSSBytes: 300, DiskReadBps: 10},
		{Name: "slack", CPUPercent: 2, MemoryRSSBytes: 200, DiskReadBps: 90},
	}
	sortApps(apps, "memory")
	if apps[0].Name != "firefox" {
		t.Errorf("memory sort first = %s", apps[0].Name)
	}
	sortApps(apps, "disk")
	if apps[0].Name != "slack" {
		t.Errorf("disk sort first = %s", apps[0].Name)
	}
	sortApps(apps, "cpu")
	if apps[0].Name != "firefox" {
		t.Errorf("cpu sort first = %s", apps[0].Name)
	}
	got := filterApps(apps, "fir")
	if len(got) != 1 || got[0].Name != "firefox" {
		t.Errorf("filter = %+v", got)
	}
	if len(filterApps(apps, "")) != 3 {
		t.Error("empty filter must return all")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[uint64]string{
		512:                    "512 B",
		2048:                   "2.0 KB",
		5 * 1024 * 1024:        "5.0 MB",
		3 * 1024 * 1024 * 1024: "3.0 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Minute:             "45m",
		2*time.Hour + 14*time.Minute: "02h 14m",
		3*24*time.Hour + 2*time.Hour: "3d 2h",
		90 * time.Second:             "1m 30s",
	}
	for in, want := range cases {
		if got := formatUptime(in); got != want {
			t.Errorf("formatUptime(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSparklineEmpty(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
}

func TestSparklineConstant(t *testing.T) {
	got := sparkline([]float64{5, 5, 5, 5})
	runes := []rune(got)
	for i, r := range runes {
		if r != '▄' {
			t.Errorf("rune %d = %q, want ▄ for constant series", i, r)
		}
	}
}

func TestSparklineMonotonic(t *testing.T) {
	got := sparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7})
	runes := []rune(got)
	if runes[0] != '▁' {
		t.Errorf("first = %q, want ▁", runes[0])
	}
	if runes[len(runes)-1] != '█' {
		t.Errorf("last = %q, want █", runes[len(runes)-1])
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] < runes[i-1] {
			t.Errorf("rune %d decreased (%q < %q)", i, runes[i], runes[i-1])
		}
	}
}

func TestParseHistoryDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"1h":  time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseHistoryDuration(in)
		if err != nil {
			t.Fatalf("parseHistoryDuration(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("parseHistoryDuration(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseHistoryDuration("3w"); err == nil {
		t.Error("expected error for unsupported duration")
	}
}

func TestBucketSecondsFor(t *testing.T) {
	if got := bucketSecondsFor(time.Hour, 60); got != 60 {
		t.Errorf("1h/60 = %d, want 60", got)
	}
	if got := bucketSecondsFor(24*time.Hour, 60); got != 1440 {
		t.Errorf("24h/60 = %d, want 1440", got)
	}
	if got := bucketSecondsFor(-time.Hour, 60); got != 1 {
		t.Errorf("negative = %d, want 1", got)
	}
}

func TestPrimaryPID(t *testing.T) {
	if got := primaryPID([]int{300, 100, 200}); got != 100 {
		t.Errorf("primaryPID = %d, want 100", got)
	}
	if got := primaryPID(nil); got != 0 {
		t.Errorf("primaryPID(nil) = %d, want 0", got)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"top", "inspect", "daemon", "history"} {
		found := false
		for _, c := range root.Commands() {
			if c.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("root missing subcommand %q", name)
		}
	}
}
