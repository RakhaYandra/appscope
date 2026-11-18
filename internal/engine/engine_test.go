package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, root, rel string, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseProcStatTotalJiffies(t *testing.T) {
	content := "cpu  100 0 50 800 10 0 5 0 0 0\ncpu0 50 0 25 400 5 0 2 0 0 0\n"
	total, idle, err := parseProcStat(content)
	if err != nil {
		t.Fatal(err)
	}
	if total != 965 {
		t.Errorf("total = %d, want 965", total)
	}
	if idle != 810 {
		t.Errorf("idle = %d, want 810 (idle+iowait)", idle)
	}
}

func TestParseProcessStatWithSpacesInComm(t *testing.T) {
	line := "2451 (Web Content 1) S 2400 2451 2451 0 -1 4194304 1000 0 0 0 123 45 0 0 20 0 11 0 999 1000000 500 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
	st, err := parseProcessStat(2451, line)
	if err != nil {
		t.Fatal(err)
	}
	if st.Comm != "Web Content 1" {
		t.Errorf("comm = %q", st.Comm)
	}
	if st.PPID != 2400 {
		t.Errorf("ppid = %d, want 2400", st.PPID)
	}
	if st.UTime != 123 || st.STime != 45 {
		t.Errorf("utime/stime = %d/%d, want 123/45", st.UTime, st.STime)
	}
	if st.Threads != 11 {
		t.Errorf("threads = %d, want 11", st.Threads)
	}
	if st.State != "S" {
		t.Errorf("state = %q, want S", st.State)
	}
	if st.StartTimeTicks != 999 {
		t.Errorf("starttime = %d, want 999 (field 22)", st.StartTimeTicks)
	}
}

func TestParseBtime(t *testing.T) {
	content := "cpu  100 0 50 800 10 0 5 0 0 0\nbtime 1700000000\n"
	btime, err := parseBtime(content)
	if err != nil {
		t.Fatal(err)
	}
	if btime != 1700000000 {
		t.Errorf("btime = %d", btime)
	}
	if _, err := parseBtime("cpu  1 2 3\n"); err == nil {
		t.Error("expected error when btime line missing")
	}
}

func TestParseProcessStatus(t *testing.T) {
	content := "Name:\tfirefox\nState:\tS (sleeping)\nUid:\t1000\t1000\t1000\t1000\nVmSize:\t 7000000 kB\nVmRSS:\t 2500000 kB\nThreads:\t42\n"
	vmRSS, vmSize, threads, uid, err := parseProcessStatus(content)
	if err != nil {
		t.Fatal(err)
	}
	if vmRSS != 2500000*1024 {
		t.Errorf("rss = %d", vmRSS)
	}
	if vmSize != 7000000*1024 {
		t.Errorf("vms = %d", vmSize)
	}
	if threads != 42 {
		t.Errorf("threads = %d", threads)
	}
	if uid != 1000 {
		t.Errorf("uid = %d, want 1000", uid)
	}
}

func TestParseProcessIO(t *testing.T) {
	content := "rchar: 1000\nwchar: 2000\nread_bytes: 14200000\nwrite_bytes: 2100000\n"
	readB, writeB, err := parseProcessIO(content)
	if err != nil {
		t.Fatal(err)
	}
	if readB != 14200000 || writeB != 2100000 {
		t.Errorf("io = %d/%d", readB, writeB)
	}
}

func TestParseMeminfo(t *testing.T) {
	content := "MemTotal:       16384000 kB\nMemFree:         1000000 kB\nMemAvailable:    8192000 kB\n"
	total, avail, err := parseMeminfo(content)
	if err != nil {
		t.Fatal(err)
	}
	if total != 16384000*1024 {
		t.Errorf("total = %d", total)
	}
	if avail != 8192000*1024 {
		t.Errorf("avail = %d", avail)
	}
}

func TestCollectorComputesCPUAndDiskRates(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "proc/stat", "cpu  100 0 50 800 10 0 5 0 0 0\n")
	writeFile(t, root, "proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\n")
	writeFile(t, root, "proc/1234/stat", "1234 (firefox) S 1 1234 1234 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 4 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile(t, root, "proc/1234/status", "VmSize:\t 7000000 kB\nVmRSS:\t 2500000 kB\nThreads:\t4\n")
	writeFile(t, root, "proc/1234/io", "read_bytes: 1000000\nwrite_bytes: 500000\n")

	c := &Collector{Root: root, NumCores: 16, now: func() time.Time { return time.Unix(1000, 0) }}

	first, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, root, "proc/stat", "cpu  200 0 100 1600 20 0 10 0 0 0\n")
	writeFile(t, root, "proc/1234/stat", "1234 (firefox) S 1 1234 1234 0 -1 4194304 100 0 0 0 100 55 0 0 20 0 4 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile(t, root, "proc/1234/io", "read_bytes: 3000000\nwrite_bytes: 1500000\n")
	c.now = func() time.Time { return time.Unix(1002, 0) }

	second, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}

	if len(second.Processes) != 1 {
		t.Fatalf("processes = %d", len(second.Processes))
	}
	p := second.Processes[0]
	deltaProc := uint64((100 + 55) - (50 + 25))
	deltaSys := uint64(1930 - 965)
	wantCPU := (float64(deltaProc) / float64(deltaSys) * 100.0) / 16.0
	if p.CPUPercent < wantCPU-0.01 || p.CPUPercent > wantCPU+0.01 {
		t.Errorf("cpu = %f, want %f", p.CPUPercent, wantCPU)
	}
	if p.DiskReadBps != 1000000 {
		t.Errorf("read bps = %d, want 1000000", p.DiskReadBps)
	}
	if p.DiskWriteBps != 500000 {
		t.Errorf("write bps = %d, want 500000", p.DiskWriteBps)
	}
	if p.MemoryRSSBytes != 2500000*1024 {
		t.Errorf("rss = %d", p.MemoryRSSBytes)
	}
	if p.Threads != 4 {
		t.Errorf("threads = %d", p.Threads)
	}

	if first.System.MemoryTotalBytes != 16000000*1024 {
		t.Errorf("mem total = %d", first.System.MemoryTotalBytes)
	}
	if second.System.ActiveProcesses != 1 {
		t.Errorf("active = %d", second.System.ActiveProcesses)
	}
	if second.System.CPUPercent <= 0 {
		t.Errorf("system cpu = %f, want > 0", second.System.CPUPercent)
	}
}

func TestCollectorSkipsVanishedProcess(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "proc/stat", "cpu  100 0 50 800 10 0 5 0 0 0\n")
	writeFile(t, root, "proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\n")
	writeFile(t, root, "proc/1234/stat", "1234 (doomed) S 1 1234 1234 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 4 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile(t, root, "proc/1234/status", "VmSize:\t 100 kB\nVmRSS:\t 50 kB\nThreads:\t1\n")
	writeFile(t, root, "proc/1234/io", "read_bytes: 100\nwrite_bytes: 100\n")

	c := &Collector{Root: root, NumCores: 4, now: func() time.Time { return time.Unix(1000, 0) }}
	if _, err := c.Sample(); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, "proc/1234")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "proc/stat", "cpu  100 0 50 900 10 0 5 0 0 0\n")
	c.now = func() time.Time { return time.Unix(1001, 0) }

	s, err := c.Sample()
	if err != nil {
		t.Fatalf("vanished pid must not error: %v", err)
	}
	if len(s.Processes) != 0 {
		t.Errorf("processes = %d, want 0", len(s.Processes))
	}
}

func TestCollectorToleratesUnreadableIO(t *testing.T) {
	root := t.TempDir()
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
	writeFile("proc/1234/stat", "1234 (firefox) S 1 1234 1234 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 4 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile("proc/1234/status", "VmSize:\t 100 kB\nVmRSS:\t 50 kB\nThreads:\t1\n")
	writeFile("proc/1234/io", "read_bytes: 100\nwrite_bytes: 100\n")
	if err := os.Chmod(filepath.Join(root, "proc/1234/io"), 0o000); err != nil {
		t.Skipf("chmod not permitted: %v", err)
	}

	c := &Collector{Root: root, NumCores: 4, now: func() time.Time { return time.Unix(1000, 0) }}
	s, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Processes) != 1 {
		t.Fatalf("processes = %d, want 1 despite unreadable io", len(s.Processes))
	}
	p := s.Processes[0]
	if p.DiskReadBps != 0 || p.DiskWriteBps != 0 {
		t.Errorf("io should degrade to 0, got %d/%d", p.DiskReadBps, p.DiskWriteBps)
	}
	if p.MemoryRSSBytes != 50*1024 {
		t.Errorf("rss = %d, want 51200 (cpu/mem must survive io denial)", p.MemoryRSSBytes)
	}
}

func TestCollectorSkipsKernelThreads(t *testing.T) {
	root := t.TempDir()
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
	writeFile("proc/4242/stat", "4242 (kworker) S 0 4242 4242 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 1 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile("proc/4242/status", "VmSize:\t 0 kB\nVmRSS:\t 0 kB\nThreads:\t1\n")
	writeFile("proc/4242/io", "read_bytes: 0\nwrite_bytes: 0\n")
	writeFile("proc/4242/cmdline", "\x00")

	c := &Collector{Root: root, NumCores: 4, now: func() time.Time { return time.Unix(1000, 0) }}
	s, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Processes) != 0 {
		t.Errorf("processes = %d, want 0 (kernel thread skipped)", len(s.Processes))
	}
	if s.System.ActiveProcesses != 0 {
		t.Errorf("active = %d, want 0", s.System.ActiveProcesses)
	}
}

func TestSampleComputesStartTimeFromBtime(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("proc/stat", "cpu  100 0 50 800 10 0 5 0 0 0\nbtime 1700000000\n")
	writeFile("proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\n")
	writeFile("proc/1234/stat", "1234 (firefox) S 1 1234 1234 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 4 0 1000000 0 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile("proc/1234/status", "VmSize:\t 100 kB\nVmRSS:\t 50 kB\nThreads:\t1\nUid:\t1000\t1000\t1000\t1000\n")
	writeFile("proc/1234/io", "read_bytes: 100\nwrite_bytes: 100\n")

	c := &Collector{Root: root, NumCores: 4, now: func() time.Time { return time.Unix(1700002000, 0) }}
	s, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}
	p := s.Processes[0]
	want := time.Unix(1700000000+1000000/100, 0)
	if !p.StartTime.Equal(want) {
		t.Errorf("starttime = %v, want %v", p.StartTime, want)
	}
}

func TestCollectorFirstSampleHasZeroRates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "proc/stat", "cpu  100 0 50 800 10 0 5 0 0 0\n")
	writeFile(t, root, "proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\n")
	writeFile(t, root, "proc/1234/stat", "1234 (init) S 0 1234 1234 0 -1 4194304 100 0 0 0 50 25 0 0 20 0 1 0 100 1000000 100 8000000 1000 2000 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	writeFile(t, root, "proc/1234/status", "VmSize:\t 100 kB\nVmRSS:\t 50 kB\nThreads:\t1\n")
	writeFile(t, root, "proc/1234/io", "read_bytes: 100\nwrite_bytes: 100\n")

	c := &Collector{Root: root, NumCores: 4, now: func() time.Time { return time.Unix(1000, 0) }}
	s, err := c.Sample()
	if err != nil {
		t.Fatal(err)
	}
	p := s.Processes[0]
	if p.CPUPercent != 0 || p.DiskReadBps != 0 || p.DiskWriteBps != 0 {
		t.Errorf("first sample must have zero rates, got cpu=%f r=%d w=%d", p.CPUPercent, p.DiskReadBps, p.DiskWriteBps)
	}
}
