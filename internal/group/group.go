package group

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/RakhaYandra/appscope/internal/engine"
	"github.com/RakhaYandra/appscope/internal/models"
)

type Grouper struct {
	Root        string
	DesktopDirs []string

	index *desktopIndex
}

func (g *Grouper) Group(procs []engine.ProcessMetrics) []models.AppMetrics {
	byPID := make(map[int]*engine.ProcessMetrics, len(procs))
	for i := range procs {
		cp := procs[i]
		byPID[cp.PID] = &cp
	}

	agg := map[string]*models.AppMetrics{}
	order := []string{}
	for _, p := range procs {
		name := g.resolveName(p, byPID)
		a, ok := agg[name]
		if !ok {
			a = &models.AppMetrics{AppName: name}
			agg[name] = a
			order = append(order, name)
		}
		a.PIDs = append(a.PIDs, p.PID)
		a.ProcessCount++
		a.CPUPercent += p.CPUPercent
		a.MemoryRSSBytes += p.MemoryRSSBytes
		a.MemoryVMSBytes += p.MemoryVMSBytes
		a.DiskReadBps += p.DiskReadBps
		a.DiskWriteBps += p.DiskWriteBps
	}

	sort.Strings(order)
	out := make([]models.AppMetrics, 0, len(order))
	for _, name := range order {
		a := agg[name]
		sort.Ints(a.PIDs)
		out = append(out, *a)
	}
	return out
}

func (g *Grouper) resolveName(p engine.ProcessMetrics, byPID map[int]*engine.ProcessMetrics) string {
	if name, ok := g.nameFromCgroup(p.PID); ok {
		return name
	}
	if name, ok := g.nameFromDesktop(p.PID); ok {
		return name
	}
	return treeRootComm(p, byPID)
}

func (g *Grouper) cgroupPath(pid int) string {
	root := g.Root
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, "proc", strconv.Itoa(pid), "cgroup")
}

func (g *Grouper) nameFromCgroup(pid int) (string, bool) {
	raw, err := os.ReadFile(g.cgroupPath(pid))
	if err != nil {
		return "", false
	}
	return scopeAppName(string(raw))
}

func (g *Grouper) nameFromDesktop(pid int) (string, bool) {
	root := g.Root
	if root == "" {
		root = "/"
	}
	exe, err := os.Readlink(filepath.Join(root, "proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return "", false
	}
	if g.index == nil {
		g.index = buildDesktopIndex(g.desktopDirs())
	}
	return g.index.lookup(exe)
}

func (g *Grouper) desktopDirs() []string {
	if g.DesktopDirs != nil {
		return g.DesktopDirs
	}
	dirs := []string{"/usr/share/applications", "/usr/local/share/applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "applications"))
	}
	return dirs
}

func treeRootComm(p engine.ProcessMetrics, byPID map[int]*engine.ProcessMetrics) string {
	cur := p
	seen := map[int]bool{cur.PID: true}
	for {
		parent, ok := byPID[cur.PPID]
		if !ok || seen[parent.PID] {
			break
		}
		seen[parent.PID] = true
		cur = *parent
	}
	if cur.Comm == "systemd" {
		return "system"
	}
	if cur.Comm == "" {
		return "pid-" + strconv.Itoa(p.PID)
	}
	return cur.Comm
}
