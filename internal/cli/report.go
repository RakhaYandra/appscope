package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/store"
)

type reportSystem struct {
	AvgCPU             float64 `json:"avg_cpu_percent"`
	MaxCPU             float64 `json:"max_cpu_percent"`
	AvgMemoryUsedBytes uint64  `json:"avg_memory_used_bytes"`
	MaxMemoryUsedBytes uint64  `json:"max_memory_used_bytes"`
	AvgDiskReadBps     uint64  `json:"avg_disk_read_bps"`
	AvgDiskWriteBps    uint64  `json:"avg_disk_write_bps"`
}

type reportApplication struct {
	AppName           string  `json:"app_name"`
	AvgCPUPercent     float64 `json:"avg_cpu_percent"`
	MaxCPUPercent     float64 `json:"max_cpu_percent"`
	AvgMemoryRSSBytes uint64  `json:"avg_memory_rss_bytes"`
	MaxMemoryRSSBytes uint64  `json:"max_memory_rss_bytes"`
	Samples           int     `json:"samples"`
}

type report struct {
	GeneratedAt  time.Time           `json:"generated_at"`
	Duration     string              `json:"duration"`
	System       reportSystem        `json:"system"`
	Applications []reportApplication `json:"applications"`
}

func buildReport(st *store.Store, since time.Time, d time.Duration) (report, error) {
	var rep report
	rep.GeneratedAt = time.Now().UTC()
	rep.Duration = durationStr2(d)

	sysRows, err := st.SystemMetricsSeries(since, int(d.Seconds()))
	if err != nil {
		return rep, err
	}
	if len(sysRows) > 0 {
		s := sysRows[0]
		rep.System = reportSystem{
			AvgCPU:             s.AvgCPU,
			MaxCPU:             s.MaxCPU,
			AvgMemoryUsedBytes: s.AvgMemoryUsed,
			MaxMemoryUsedBytes: s.MaxMemoryUsed,
			AvgDiskReadBps:     s.AvgDiskReadBps,
			AvgDiskWriteBps:    s.AvgDiskWriteBps,
		}
	}

	appRows, err := st.AppMetricsAggregate(since)
	if err != nil {
		return rep, err
	}
	rep.Applications = make([]reportApplication, 0, len(appRows))
	for _, a := range appRows {
		rep.Applications = append(rep.Applications, reportApplication{
			AppName:           a.AppName,
			AvgCPUPercent:     a.AvgCPU,
			MaxCPUPercent:     a.MaxCPU,
			AvgMemoryRSSBytes: a.AvgRSS,
			MaxMemoryRSSBytes: a.MaxRSS,
			Samples:           a.Samples,
		})
	}
	return rep, nil
}

func renderReportJSON(w io.Writer, rep report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func renderReportCSV(w io.Writer, rep report) error {
	if _, err := fmt.Fprintln(w, "[system]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "generated_at,duration,avg_cpu_percent,max_cpu_percent,avg_memory_used_bytes,max_memory_used_bytes,avg_disk_read_bps,avg_disk_write_bps"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s,%s,%.4f,%.4f,%d,%d,%d,%d\n",
		rep.GeneratedAt.Format(time.RFC3339), rep.Duration,
		rep.System.AvgCPU, rep.System.MaxCPU,
		rep.System.AvgMemoryUsedBytes, rep.System.MaxMemoryUsedBytes,
		rep.System.AvgDiskReadBps, rep.System.AvgDiskWriteBps); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "[applications]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "app_name,avg_cpu_percent,max_cpu_percent,avg_memory_rss_bytes,max_memory_rss_bytes,samples"); err != nil {
		return err
	}
	for _, a := range rep.Applications {
		if _, err := fmt.Fprintf(w, "%s,%.4f,%.4f,%d,%d,%d\n",
			a.AppName, a.AvgCPUPercent, a.MaxCPUPercent,
			a.AvgMemoryRSSBytes, a.MaxMemoryRSSBytes, a.Samples); err != nil {
			return err
		}
	}
	return nil
}

func newReportCmd() *cobra.Command {
	var durationStr, format, output string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Export a resource usage report (JSON or CSV)",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseHistoryDuration(durationStr)
			if err != nil {
				return err
			}
			if format != "json" && format != "csv" {
				return fmt.Errorf("unsupported format %q (want json|csv)", format)
			}
			if _, err := os.Stat(dbPath); err != nil {
				return fmt.Errorf("no history database at %s (is appscoped running?)", dbPath)
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			rep, err := buildReport(st, time.Now().Add(-d), d)
			if err != nil {
				return err
			}

			var w = cmd.OutOrStdout()
			var f *os.File
			if output != "" && output != "-" {
				var err error
				f, err = os.Create(output)
				if err != nil {
					return fmt.Errorf("create %s: %w", output, err)
				}
				defer func() { _ = f.Close() }()
				w = f
			}
			if format == "json" {
				return renderReportJSON(w, rep)
			}
			return renderReportCSV(w, rep)
		},
	}
	cmd.Flags().StringVar(&durationStr, "duration", "24h", "time range: 1h|24h|7d")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json|csv")
	cmd.Flags().StringVar(&output, "output", "-", "output file (default stdout)")
	cmd.Flags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	return cmd
}
