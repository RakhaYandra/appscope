package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RakhaYandra/appscope/internal/engine"
	"github.com/RakhaYandra/appscope/internal/group"
	"github.com/RakhaYandra/appscope/internal/store"
)

func writeFakeProc(t *testing.T, root string) {
	t.Helper()
	writeFile := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("proc/stat", "cpu  100 0 50 800 10 0 5 0 0 0\n")
	writeFile("proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\n")
	writeFile("proc/4242/stat", "4242 (myapp) S 1 4242 4242 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 4 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile("proc/4242/status", "VmSize:\t 7000000 kB\nVmRSS:\t 2500000 kB\nThreads:\t4\n")
	writeFile("proc/4242/io", "read_bytes: 1000000\nwrite_bytes: 500000\n")
	writeFile("proc/4242/cgroup", "0::/user.slice\n")
}

func TestTickPublishesSnapshotAndPersists(t *testing.T) {
	root := t.TempDir()
	writeFakeProc(t, root)
	dbPath := filepath.Join(t.TempDir(), "appscope.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d := &Daemon{
		Store:     st,
		Collector: &engine.Collector{Root: root, NumCores: 8},
		Grouper:   &group.Grouper{Root: root, DesktopDirs: []string{t.TempDir()}},
		Interval:  time.Second,
	}
	if err := d.Tick(); err != nil {
		t.Fatal(err)
	}
	snap := d.Snapshot()
	if snap.System.ActiveProcesses != 1 {
		t.Errorf("active = %d, want 1", snap.System.ActiveProcesses)
	}
	if len(snap.Apps) != 1 || snap.Apps[0].AppName != "myapp" {
		t.Fatalf("apps = %+v, want one app 'myapp' via ppid-tree fallback", snap.Apps)
	}

	rows, err := st.AppMetricsSince("myapp", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("db rows = %d, want 1", len(rows))
	}
	if _, err := st.LatestSystemMetrics(); err != nil {
		t.Errorf("latest system metrics: %v", err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	root := t.TempDir()
	writeFakeProc(t, root)
	dbPath := filepath.Join(t.TempDir(), "appscope.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d := &Daemon{
		Store:     st,
		Collector: &engine.Collector{Root: root, NumCores: 8},
		Grouper:   &group.Grouper{Root: root, DesktopDirs: []string{t.TempDir()}},
		Interval:  20 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	time.Sleep(70 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	if snap := d.Snapshot(); snap.System.ActiveProcesses != 1 {
		t.Errorf("active = %d, want 1 after ticks", snap.System.ActiveProcesses)
	}
}
