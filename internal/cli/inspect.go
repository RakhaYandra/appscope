package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

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

	printInspection(w, *target, v.Source)
	if showTree {
		printProcessTree(w, target.PIDs)
	}
	return nil
}

func printInspection(w io.Writer, a appView, source string) {
	fmt.Fprintf(w, "AppScope Application Inspection — %s\n", a.Name)
	fmt.Fprintf(w, "Target: %s (Grouped Application) | Source: %s\n", a.Name, source)
	fmt.Fprintf(w, "Primary PID: %d | Processes: %d\n\n", primaryPID(a.PIDs), a.ProcessCount)
	fmt.Fprintf(w, "RESOURCE CONSUMPTION\n")
	fmt.Fprintf(w, "  CPU Usage         %.1f%%\n", a.CPUPercent)
	fmt.Fprintf(w, "  Memory (RSS)      %s\n", humanBytes(a.MemoryRSSBytes))
	fmt.Fprintf(w, "  Memory (VMS)      %s\n", humanBytes(a.MemoryVMSBytes))
	fmt.Fprintf(w, "  Disk I/O Rate     Read: %s/s | Write: %s/s\n",
		humanBytes(a.DiskReadBps), humanBytes(a.DiskWriteBps))
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
		fmt.Fprintf(w, "\nPROCESS TREE\n  (unavailable: %v)\n", err)
		return
	}
	byPID := map[int]engine.ProcessMetrics{}
	for _, p := range sample.Processes {
		byPID[p.PID] = p
	}

	fmt.Fprintf(w, "\nPROCESS TREE (%d processes)\n", len(appPIDs))
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
		fmt.Fprintf(w, "%s%s (%d)\n", indent, proc.Comm, proc.PID)
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
