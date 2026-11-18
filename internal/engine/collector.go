package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/RakhaYandra/appscope/internal/models"
)

type ProcessMetrics struct {
	PID            int
	PPID           int
	Comm           string
	CPUPercent     float64
	MemoryRSSBytes uint64
	MemoryVMSBytes uint64
	DiskReadBps    uint64
	DiskWriteBps   uint64
	Threads        int
}

type Sample struct {
	Timestamp time.Time
	System    models.SystemMetrics
	Processes []ProcessMetrics
}

type processBaseline struct {
	jiffies    uint64
	readBytes  uint64
	writeBytes uint64
}

type Collector struct {
	Root     string
	NumCores int

	now func() time.Time

	prevSysJiffies uint64
	prevIdle       uint64
	prevProc       map[int]processBaseline
	prevTime       time.Time
	hasBaseline    bool
}

func NewCollector() *Collector {
	return &Collector{
		Root:     "/",
		NumCores: runtime.NumCPU(),
		now:      time.Now,
	}
}

func (c *Collector) Sample() (*Sample, error) {
	if c.now == nil {
		c.now = time.Now
	}
	if c.NumCores <= 0 {
		c.NumCores = runtime.NumCPU()
	}
	root := c.Root
	if root == "" {
		root = "/"
	}
	now := c.now()

	statRaw, err := os.ReadFile(filepath.Join(root, "proc", "stat"))
	if err != nil {
		return nil, fmt.Errorf("read procfs stat: %w", err)
	}
	sysJiffies, sysIdle, err := parseProcStat(string(statRaw))
	if err != nil {
		return nil, err
	}

	memRaw, err := os.ReadFile(filepath.Join(root, "proc", "meminfo"))
	if err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}
	memTotal, memAvail, err := parseMeminfo(string(memRaw))
	if err != nil {
		return nil, err
	}

	pids, err := listPIDs(filepath.Join(root, "proc"))
	if err != nil {
		return nil, err
	}

	procs := make([]ProcessMetrics, 0, len(pids))
	cur := make(map[int]processBaseline, len(pids))

	var sysDiskRead, sysDiskWrite uint64
	for _, pid := range pids {
		pm, ok := c.readProcess(root, pid, now, sysJiffies)
		if !ok {
			continue
		}
		procs = append(procs, pm.metrics)
		cur[pid] = pm.baseline
		sysDiskRead += pm.metrics.DiskReadBps
		sysDiskWrite += pm.metrics.DiskWriteBps
	}

	sample := &Sample{
		Timestamp: now,
		System: models.SystemMetrics{
			Timestamp:        now,
			MemoryUsedBytes:  memTotal - memAvail,
			MemoryTotalBytes: memTotal,
			DiskReadBps:      sysDiskRead,
			DiskWriteBps:     sysDiskWrite,
			ActiveProcesses:  len(procs),
		},
		Processes: procs,
	}
	if c.hasBaseline && sysJiffies > c.prevSysJiffies {
		dTotal := sysJiffies - c.prevSysJiffies
		dIdle := sysIdle - c.prevIdle
		sample.System.CPUPercent = float64(dTotal-dIdle) / float64(dTotal) * 100.0
	}

	c.prevSysJiffies = sysJiffies
	c.prevIdle = sysIdle
	c.prevProc = cur
	c.prevTime = now
	c.hasBaseline = true
	return sample, nil
}

type processRead struct {
	metrics  ProcessMetrics
	baseline processBaseline
}

func (c *Collector) readProcess(root string, pid int, now time.Time, sysJiffies uint64) (processRead, bool) {
	var out processRead
	dir := filepath.Join(root, "proc", strconv.Itoa(pid))

	statRaw, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return out, false
	}
	pstat, err := parseProcessStat(pid, string(statRaw))
	if err != nil {
		return out, false
	}

	statusRaw, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return out, false
	}
	vmRSS, vmSize, _, err := parseProcessStatus(string(statusRaw))
	if err != nil {
		return out, false
	}

	ioRaw, err := os.ReadFile(filepath.Join(dir, "io"))
	if err != nil {
		return out, false
	}
	readB, writeB, err := parseProcessIO(string(ioRaw))
	if err != nil {
		return out, false
	}

	jiffies := pstat.UTime + pstat.STime
	out.metrics = ProcessMetrics{
		PID:            pid,
		PPID:           pstat.PPID,
		Comm:           pstat.Comm,
		MemoryRSSBytes: vmRSS,
		MemoryVMSBytes: vmSize,
		Threads:        pstat.Threads,
	}
	out.baseline = processBaseline{jiffies: jiffies, readBytes: readB, writeBytes: writeB}

	if c.hasBaseline {
		if prev, ok := c.prevProc[pid]; ok {
			dt := now.Sub(c.prevTime).Seconds()
			if dSys := sysJiffies - c.prevSysJiffies; dSys > 0 && jiffies >= prev.jiffies {
				dProc := jiffies - prev.jiffies
				out.metrics.CPUPercent = (float64(dProc) / float64(dSys) * 100.0) / float64(c.NumCores)
			}
			if dt > 0 {
				if readB >= prev.readBytes {
					out.metrics.DiskReadBps = uint64(float64(readB-prev.readBytes) / dt)
				}
				if writeB >= prev.writeBytes {
					out.metrics.DiskWriteBps = uint64(float64(writeB-prev.writeBytes) / dt)
				}
			}
		}
	}
	return out, true
}

func listPIDs(procDir string) ([]int, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", procDir, err)
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
