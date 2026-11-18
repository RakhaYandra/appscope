package cli

import (
	"fmt"
	"io"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/engine"
)

func newInspectCmd(socketPath *string) *cobra.Command {
	var pidFlag int
	var treeFlag bool
	cmd := &cobra.Command{
		Use:   "inspect [app]",
		Short: "Deep inspection of an application or PID",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			if name == "" && pidFlag == 0 {
				return fmt.Errorf("provide an application name or --pid")
			}
			return runInspect(cmd.OutOrStdout(), *socketPath, name, pidFlag, treeFlag)
		},
	}
	cmd.Flags().IntVar(&pidFlag, "pid", 0, "inspect the app containing this PID")
	cmd.Flags().BoolVar(&treeFlag, "tree", true, "print process tree")
	return cmd
}

func runInspect(w io.Writer, socketPath, name string, pidFlag int, showTree bool) error {
	v := fetchView(socketPath, 100)
	var target *appView
	if pidFlag != 0 {
		for i := range v.Apps {
			for _, p := range v.Apps[i].PIDs {
				if p == pidFlag {
					target = &v.Apps[i]
				}
			}
		}
		if target == nil {
			return fmt.Errorf("no application found containing pid %d", pidFlag)
		}
	} else {
		q := strings.ToLower(name)
		for i := range v.Apps {
			if strings.ToLower(v.Apps[i].Name) == q {
				target = &v.Apps[i]
				break
			}
		}
		if target == nil {
			matches := filterApps(v.Apps, q)
			if len(matches) == 0 {
				return fmt.Errorf("application not found: %s (source: %s)", name, v.Source)
			}
			target = &matches[0]
		}
	}

	extra := gatherLocalExtras(target.PIDs)
	printInspection(w, *target, v.Source, extra)
	if showTree {
		printProcessTree(w, target.PIDs)
	}
	return nil
}

type inspectExtra struct {
	user    string
	state   string
	uptime  string
	threads int
}

func gatherLocalExtras(pids []int) inspectExtra {
	var extra inspectExtra
	collector := engine.NewCollector()
	sample, err := collector.Sample()
	if err != nil {
		return extra
	}
	primary := primaryPID(pids)
	pidSet := make(map[int]bool, len(pids))
	for _, pid := range pids {
		pidSet[pid] = true
	}
	for _, p := range sample.Processes {
		if !pidSet[p.PID] {
			continue
		}
		extra.threads += p.Threads
		if p.PID != primary {
			continue
		}
		if p.State != "" {
			extra.state = p.State
		}
		if p.Uid >= 0 {
			if u, err := user.LookupId(strconv.Itoa(p.Uid)); err == nil {
				extra.user = u.Username
			} else {
				extra.user = strconv.Itoa(p.Uid)
			}
		}
		if !p.StartTime.IsZero() {
			extra.uptime = formatUptime(time.Since(p.StartTime))
		}
	}
	return extra
}

func printInspection(w io.Writer, a appView, source string, extra inspectExtra) {
	_, _ = fmt.Fprintf(w, "AppScope Application Inspection — %s\n", a.Name)
	_, _ = fmt.Fprintf(w, "Target: %s (Grouped Application) | Source: %s\n", a.Name, source)
	details := []string{fmt.Sprintf("Primary PID: %d", primaryPID(a.PIDs))}
	if extra.state != "" {
		details = append(details, "Status: "+extra.state)
	}
	if extra.user != "" {
		details = append(details, "User: "+extra.user)
	}
	if extra.uptime != "" {
		details = append(details, "Uptime: "+extra.uptime)
	}
	details = append(details, fmt.Sprintf("Processes: %d", a.ProcessCount))
	_, _ = fmt.Fprintf(w, "%s\n\n", strings.Join(details, " | "))
	_, _ = fmt.Fprintf(w, "RESOURCE CONSUMPTION\n")
	_, _ = fmt.Fprintf(w, "  CPU Usage         %.1f%%\n", a.CPUPercent)
	_, _ = fmt.Fprintf(w, "  Memory (RSS)      %s\n", humanBytes(a.MemoryRSSBytes))
	_, _ = fmt.Fprintf(w, "  Memory (VMS)      %s\n", humanBytes(a.MemoryVMSBytes))
	if extra.threads > 0 {
		_, _ = fmt.Fprintf(w, "  Threads           %d\n", extra.threads)
	}
	_, _ = fmt.Fprintf(w, "  Disk I/O Rate     Read: %s/s | Write: %s/s\n",
		humanBytes(a.DiskReadBps), humanBytes(a.DiskWriteBps))
}

func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	days, hours, mins, secs := s/86400, (s%86400)/3600, (s%3600)/60, s%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%02dh %02dm", hours, mins)
	case s < 120:
		return fmt.Sprintf("%dm %ds", mins, secs)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func primaryPID(pids []int) int {
	if len(pids) == 0 {
		return 0
	}
	minPID := pids[0]
	for _, p := range pids {
		if p < minPID {
			minPID = p
		}
	}
	return minPID
}

func printProcessTree(w io.Writer, appPIDs []int) {
	if len(appPIDs) == 0 {
		return
	}
	inSet := make(map[int]bool, len(appPIDs))
	for _, p := range appPIDs {
		inSet[p] = true
	}

	collector := engine.NewCollector()
	sample, err := collector.Sample()
	if err != nil {
		_, _ = fmt.Fprintf(w, "\nPROCESS TREE\n  (unavailable: %v)\n", err)
		return
	}
	byPID := map[int]engine.ProcessMetrics{}
	for _, p := range sample.Processes {
		byPID[p.PID] = p
	}

	_, _ = fmt.Fprintf(w, "\nPROCESS TREE (%d processes)\n", len(appPIDs))
	sort.Ints(appPIDs)
	roots := make([]int, 0)
	for _, p := range appPIDs {
		proc, ok := byPID[p]
		if !ok || !inSet[proc.PPID] {
			roots = append(roots, p)
		}
	}
	printed := map[int]bool{}
	var printTreeNode func(pid, depth int)
	printTreeNode = func(pid int, depth int) {
		proc, ok := byPID[pid]
		if !ok {
			return
		}
		indent := strings.Repeat("  ", depth)
		_, _ = fmt.Fprintf(w, "%s%s (%d)\n", indent, proc.Comm, proc.PID)
		printed[pid] = true
		children := make([]int, 0)
		for _, p := range appPIDs {
			if byPID[p].PPID == pid {
				children = append(children, p)
			}
		}
		sort.Ints(children)
		for _, c := range children {
			printTreeNode(c, depth+1)
		}
	}
	for _, r := range roots {
		printTreeNode(r, 0)
	}
	for _, p := range appPIDs {
		if !printed[p] {
			printTreeNode(p, 0)
		}
	}
}
