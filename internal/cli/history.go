package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/store"
)

var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

const historyBuckets = 60

func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	out := make([]rune, len(values))
	if max-min < 1e-9 {
		for i := range out {
			out[i] = '▄'
		}
		return string(out)
	}
	scale := float64(len(sparkBlocks)-1) / (max - min)
	for i, v := range values {
		idx := int((v - min) * scale)
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		out[i] = sparkBlocks[idx]
	}
	return string(out)
}

func parseHistoryDuration(s string) (time.Duration, error) {
	switch s {
	case "1h":
		return time.Hour, nil
	case "24h":
		return 24 * time.Hour, nil
	case "7d":
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported duration %q (want 1h|24h|7d)", s)
	}
}

func bucketSecondsFor(d time.Duration, buckets int) int {
	if d <= 0 || buckets <= 0 {
		return 1
	}
	b := int(d.Seconds()) / buckets
	if b < 1 {
		return 1
	}
	return b
}

type historyPoint struct {
	bucket time.Time
	value  float64
}

func newHistoryCmd() *cobra.Command {
	var durationStr, metric string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "history [app]",
		Short: "Historical resource usage trend (sparkline)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseHistoryDuration(durationStr)
			if err != nil {
				return err
			}
			app := ""
			if len(args) == 1 {
				app = args[0]
			}
			switch metric {
			case "cpu", "memory", "disk":
			default:
				return fmt.Errorf("unknown metric %q (want cpu|memory|disk)", metric)
			}
			return runHistory(cmd.OutOrStdout(), dbPath, app, d, metric)
		},
	}
	cmd.Flags().StringVar(&durationStr, "duration", "24h", "time range: 1h|24h|7d")
	cmd.Flags().StringVar(&metric, "metric", "cpu", "metric: cpu|memory|disk")
	cmd.Flags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	return cmd
}

func runHistory(w io.Writer, dbPath, app string, d time.Duration, metric string) error {
	if _, err := os.Stat(dbPath); err != nil {
		if app == "" {
			return fmt.Errorf("no history database at %s (is appscoped running?)", dbPath)
		}
		return fmt.Errorf("no history database at %s", dbPath)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	since := time.Now().Add(-d)
	bucket := bucketSecondsFor(d, historyBuckets)

	var points []historyPoint
	if app == "" {
		rows, err := st.SystemMetricsSeries(since, bucket)
		if err != nil {
			return err
		}
		for _, r := range rows {
			points = append(points, historyPoint{r.BucketStart, systemMetricValue(r, metric)})
		}
	} else {
		rows, err := st.AppMetricsSeries(app, since, bucket)
		if err != nil {
			return err
		}
		for _, r := range rows {
			points = append(points, historyPoint{r.BucketStart, appMetricValue(r, metric)})
		}
	}
	if len(points) == 0 {
		target := "system"
		if app != "" {
			target = app
		}
		fmt.Fprintf(w, "No history for %s in the last %s (is appscoped running?)\n", target, durationStr2(d))
		return nil
	}

	values := make([]float64, len(points))
	var peak int
	for i, p := range points {
		values[i] = p.value
		if values[i] > values[peak] {
			peak = i
		}
	}
	minV, maxV, sum := values[0], values[0], 0.0
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += v
	}
	avgV := sum / float64(len(values))
	current := values[len(values)-1]

	target := "System"
	if app != "" {
		target = app
	}
	_, _ = fmt.Fprintf(w, "AppScope History — %s over last %s (metric: %s, bucket %s)\n",
		target, durationStr2(d), metric, (time.Duration(bucket) * time.Second).Round(time.Second))
	_, _ = fmt.Fprintf(w, "%s\n", sparkline(values))
	_, _ = fmt.Fprintf(w, "min %s | avg %s | max %s | current %s\n",
		formatHistoryValue(minV, metric), formatHistoryValue(avgV, metric),
		formatHistoryValue(maxV, metric), formatHistoryValue(current, metric))
	peakLabel := "15:04"
	if d > 48*time.Hour {
		peakLabel = "Jan 02 15:04"
	}
	_, _ = fmt.Fprintf(w, "peak %s at %s\n", formatHistoryValue(values[peak], metric), points[peak].bucket.Local().Format(peakLabel))
	return nil
}

func durationStr2(d time.Duration) string {
	switch {
	case d == time.Hour:
		return "1h"
	case d == 24*time.Hour:
		return "24h"
	case d == 7*24*time.Hour:
		return "7d"
	default:
		return d.String()
	}
}

func systemMetricValue(r store.SystemSeriesRow, metric string) float64 {
	switch metric {
	case "memory":
		return float64(r.AvgMemoryUsed)
	case "disk":
		return float64(r.AvgDiskReadBps + r.AvgDiskWriteBps)
	default:
		return r.AvgCPU
	}
}

func appMetricValue(r store.AppSeriesRow, metric string) float64 {
	switch metric {
	case "memory":
		return float64(r.AvgRSS)
	case "disk":
		return float64(r.AvgDiskBps)
	default:
		return r.AvgCPU
	}
}

func formatHistoryValue(v float64, metric string) string {
	switch metric {
	case "memory":
		return humanBytes(uint64(v))
	case "disk":
		return fmt.Sprintf("%.1f MB/s", v/(1024*1024))
	default:
		return fmt.Sprintf("%.1f%%", v)
	}
}
