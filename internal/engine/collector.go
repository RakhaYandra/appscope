package engine

import (
	"bytes"
	"fmt"
	"log"
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
	State          string
	Uid            int
	StartTime      time.Time
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
	prevCPU        cpuStats
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
	cols, err := parseProcStatColumns(string(statRaw))
	if err != nil {
		return nil, err
	}
	sysJiffies, sysIdle := cols.Total(), cols.Idle+cols.Iowait
	bootTime, btimeErr := parseBtime(string(statRaw))

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
		pm, ok := c.readProcess(root, pid, now, sysJiffies, bootTime, btimeErr == nil)
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
		if sample.System.CPUPercent > spikeThreshold {
			log.Printf("cpu spike: %s", formatCPUSpike(c.prevCPU, cols, sample.System.CPUPercent))
		}
	}

	c.prevCPU = cols

	c.prevSysJiffies = sysJiffies
	c.prevIdle = sysIdle
	c.prevProc = cur
	c.prevTime = now
	c.hasBaseline = true
	return sample, nil
}

const spikeThreshold = 70.0

func formatCPUSpike(prev, cur cpuStats, cpuPercent float64) string {
	delta := func(a, b uint64) uint64 {
		if b >= a {
			return b - a
		}
		return 0
	}
	return fmt.Sprintf("cpu_percent=%.1f user=%d nice=%d system=%d idle=%d iowait=%d irq=%d softirq=%d steal=%d",
		cpuPercent,
		delta(prev.User, cur.User), delta(prev.Nice, cur.Nice), delta(prev.System, cur.System),
		delta(prev.Idle, cur.Idle), delta(prev.Iowait, cur.Iowait),
		delta(prev.Irq, cur.Irq), delta(prev.Softirq, cur.Softirq), delta(prev.Steal, cur.Steal))
}

type processRead struct {
	metrics  ProcessMetrics
	baseline processBaseline
}

func (c *Collector) readProcess(root string, pid int, now time.Time, sysJiffies, bootTime uint64, hasBootTime bool) (processRead, bool) {
	var out processRead
	dir := filepath.Join(root, "proc", strconv.Itoa(pid))

	// kernel threads have an empty cmdline; they are not applications.
	if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil && len(bytes.TrimRight(cmdline, "\x00")) == 0 {
		return out, false
	}

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
	vmRSS, vmSize, _, uid, err := parseProcessStatus(string(statusRaw))
	if err != nil {
		return out, false
	}

	// /proc/pid/io is owner-only and EPERM inside systemd mount namespaces;
	// degrade to zero I/O rather than dropping the process.
	var readB, writeB uint64
	if ioRaw, err := os.ReadFile(filepath.Join(dir, "io")); err == nil {
		if r, w, err := parseProcessIO(string(ioRaw)); err == nil {
			readB, writeB = r, w
		}
	}

	jiffies := pstat.UTime + pstat.STime
	out.metrics = ProcessMetrics{
		PID:            pid,
		PPID:           pstat.PPID,
		Comm:           pstat.Comm,
		State:          pstat.State,
		Uid:            uid,
		MemoryRSSBytes: vmRSS,
		MemoryVMSBytes: vmSize,
		Threads:        pstat.Threads,
	}
	// kernel USER_HZ is 100 on all tier-1 architectures
	if hasBootTime {
		out.metrics.StartTime = time.Unix(int64(bootTime)+int64(pstat.StartTimeTicks/100), 0)
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
