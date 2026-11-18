package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/ipc"
)

func NewRootCmd() *cobra.Command {
	var socketPath string

	root := &cobra.Command{
		Use:   "appscope",
		Short: "Application resource intelligence for Linux",
		Long:  "AppScope monitors CPU, memory, and disk I/O per logical application — one engine, queried by this CLI and the Omarchy plugin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			watch, _ := cmd.Flags().GetBool("watch")
			interval, _ := cmd.Flags().GetDuration("interval")
			for {
				printOverview(cmd.OutOrStdout(), fetchView(socketPath, 5))
				if !watch {
					return nil
				}
				time.Sleep(interval)
				_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J")
			}
		},
	}
	root.PersistentFlags().StringVar(&socketPath, "socket", ipc.DefaultSocketPath(), "path to appscoped socket")
	root.Flags().BoolP("watch", "w", false, "refresh continuously")
	root.Flags().Duration("interval", 2*time.Second, "refresh interval for --watch")

	root.AddCommand(
		newInspectCmd(&socketPath),
		newDaemonCmd(&socketPath),
		newTopCmd(&socketPath),
		newHistoryCmd(),
	)
	return root
}

func printOverview(w io.Writer, v view) {
	_, _ = fmt.Fprintf(w, "AppScope — System Overview\n")
	_, _ = fmt.Fprintf(w, "Source: %s\n\n", v.Source)
	s := v.System
	_, _ = fmt.Fprintf(w, "CPU %.1f%% | RAM %.1f/%.1f GB | Disk R %.1f MB/s W %.1f MB/s | Procs %d\n\n",
		s.CPUPercent, s.MemoryUsedGB, s.MemoryTotalGB, s.DiskReadMBps, s.DiskWriteMBps, s.ActiveProcesses)
	_, _ = fmt.Fprintf(w, "TOP APPS (by CPU)\n")
	_, _ = fmt.Fprintf(w, "  %-20s %-6s %-8s %-10s\n", "NAME", "PROCS", "CPU%", "RSS")
	for _, a := range v.Apps {
		_, _ = fmt.Fprintf(w, "  %-20s %-6d %-8.1f %-10s\n", a.Name, a.ProcessCount, a.CPUPercent, humanBytes(a.MemoryRSSBytes))
	}
	if len(v.Apps) == 0 {
		_, _ = fmt.Fprintln(w, "  (no applications found)")
	}
}
