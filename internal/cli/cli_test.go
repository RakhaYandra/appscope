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
	for _, name := range []string{"top", "inspect", "daemon"} {
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
