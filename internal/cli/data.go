package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/RakhaYandra/appscope/internal/engine"
	"github.com/RakhaYandra/appscope/internal/group"
	"github.com/RakhaYandra/appscope/internal/ipc"
	"github.com/RakhaYandra/appscope/internal/models"
)

type appView struct {
	Name           string
	PIDs           []int
	ProcessCount   int
	CPUPercent     float64
	MemoryRSSBytes uint64
	MemoryVMSBytes uint64
	DiskReadBps    uint64
	DiskWriteBps   uint64
}

type systemView struct {
	CPUPercent      float64
	MemoryUsedGB    float64
	MemoryTotalGB   float64
	DiskReadMBps    float64
	DiskWriteMBps   float64
	ActiveProcesses int
}

type view struct {
	System systemView
	Apps   []appView
	Source string
}

func fetchView(socketPath string, limit int) view {
	if v, err := viewFromSocket(socketPath, limit); err == nil {
		v.Source = "daemon"
		return v
	}
	return localView(limit)
}

func viewFromSocket(socketPath string, limit int) (view, error) {
	var v view
	resp, err := ipc.Call(socketPath, "appscope.getTopApps", map[string]interface{}{
		"limit": float64(limit),
	})
	if err != nil {
		return v, err
	}
	sys, _ := resp["system"].(map[string]interface{})
	v.System = systemView{
		CPUPercent:      num(sys["cpu_percent"]),
		MemoryUsedGB:    num(sys["memory_used_gb"]),
		MemoryTotalGB:   num(sys["memory_total_gb"]),
		DiskReadMBps:    num(sys["disk_read_mbps"]),
		DiskWriteMBps:   num(sys["disk_write_mbps"]),
		ActiveProcesses: int(num(sys["active_processes"])),
	}
	apps, _ := resp["applications"].([]interface{})
	for _, raw := range apps {
		m, _ := raw.(map[string]interface{})
		a := appView{
			Name:           str(m["app_name"]),
			ProcessCount:   int(num(m["process_count"])),
			CPUPercent:     num(m["cpu_percent"]),
			MemoryRSSBytes: uint64(num(m["memory_rss_bytes"])),
			MemoryVMSBytes: uint64(num(m["memory_vms_bytes"])),
			DiskReadBps:    uint64(num(m["disk_read_bps"])),
			DiskWriteBps:   uint64(num(m["disk_write_bps"])),
		}
		if pids, ok := m["pids"].([]interface{}); ok {
			for _, p := range pids {
				a.PIDs = append(a.PIDs, int(num(p)))
			}
		}
		v.Apps = append(v.Apps, a)
	}
	return v, nil
}

func localView(limit int) view {
	collector := engine.NewCollector()
	grouper := &group.Grouper{}

	base, err := collector.Sample()
	if err != nil {
		return view{Source: "unavailable: " + err.Error()}
	}
	time.Sleep(500 * time.Millisecond)
	second, err := collector.Sample()
	if err != nil {
		return view{Source: "unavailable: " + err.Error()}
	}
	_ = base

	apps := grouper.Group(second.Processes)
	sort.Slice(apps, func(i, j int) bool { return apps[i].CPUPercent > apps[j].CPUPercent })
	if len(apps) > limit {
		apps = apps[:limit]
	}
	v := view{Source: "local (daemon not running)"}
	v.System = systemView{
		CPUPercent:      second.System.CPUPercent,
		MemoryUsedGB:    float64(second.System.MemoryUsedBytes) / (1024 * 1024 * 1024),
		MemoryTotalGB:   float64(second.System.MemoryTotalBytes) / (1024 * 1024 * 1024),
		DiskReadMBps:    float64(second.System.DiskReadBps) / (1024 * 1024),
		DiskWriteMBps:   float64(second.System.DiskWriteBps) / (1024 * 1024),
		ActiveProcesses: second.System.ActiveProcesses,
	}
	for _, a := range apps {
		v.Apps = append(v.Apps, appFromModel(a))
	}
	return v
}

func appFromModel(a models.AppMetrics) appView {
	return appView{
		Name:           a.AppName,
		PIDs:           a.PIDs,
		ProcessCount:   a.ProcessCount,
		CPUPercent:     a.CPUPercent,
		MemoryRSSBytes: a.MemoryRSSBytes,
		MemoryVMSBytes: a.MemoryVMSBytes,
		DiskReadBps:    a.DiskReadBps,
		DiskWriteBps:   a.DiskWriteBps,
	}
}

func num(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		return 0
	}
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func sortApps(apps []appView, key string) {
	switch key {
	case "memory":
		sort.Slice(apps, func(i, j int) bool { return apps[i].MemoryRSSBytes > apps[j].MemoryRSSBytes })
	case "disk":
		sort.Slice(apps, func(i, j int) bool {
			return apps[i].DiskReadBps+apps[i].DiskWriteBps > apps[j].DiskReadBps+apps[j].DiskWriteBps
		})
	default:
		sort.Slice(apps, func(i, j int) bool { return apps[i].CPUPercent > apps[j].CPUPercent })
	}
}

func filterApps(apps []appView, query string) []appView {
	if query == "" {
		return apps
	}
	q := strings.ToLower(query)
	out := make([]appView, 0, len(apps))
	for _, a := range apps {
		if strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	return out
}
