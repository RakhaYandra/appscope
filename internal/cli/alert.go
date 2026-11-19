package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/store"
)

func validateAlertInputs(appName, metric string, threshold float64) error {
	if appName == "" {
		return fmt.Errorf("app name must not be empty (use '*' for all apps)")
	}
	if !store.ValidMetric(metric) {
		return fmt.Errorf("invalid metric %q (want cpu_percent|memory_rss_bytes|disk_read_bytes|disk_write_bytes)", metric)
	}
	if threshold <= 0 {
		return fmt.Errorf("threshold must be > 0")
	}
	return nil
}

func formatAlertRules(w io.Writer, rules []store.AlertRule) {
	_, _ = fmt.Fprintf(w, "%-4s %-20s %-18s %-14s %s\n", "ID", "APP", "METRIC", "THRESHOLD", "STATE")
	if len(rules) == 0 {
		_, _ = fmt.Fprintf(w, "(no alert rules configured)\n")
		return
	}
	for _, r := range rules {
		state := "enabled"
		if !r.Enabled {
			state = "disabled"
		}
		_, _ = fmt.Fprintf(w, "%-4d %-20s %-18s %-14.1f %s\n", r.ID, r.AppName, r.Metric, r.Threshold, state)
	}
}

func newAlertCmd() *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Manage resource threshold alerts",
	}

	set := &cobra.Command{
		Use:   "set <app|*> <metric> <threshold>",
		Short: "Create an alert rule (fires when app metric exceeds threshold)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, metric := args[0], args[1]
			threshold, err := strconv.ParseFloat(args[2], 64)
			if err != nil {
				return fmt.Errorf("invalid threshold %q: %w", args[2], err)
			}
			if err := validateAlertInputs(appName, metric, threshold); err != nil {
				return err
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			id, err := st.AddAlertRule(appName, metric, threshold)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "alert rule #%d created: %s %s > %.1f\n", id, appName, metric, threshold)
			return nil
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List configured alert rules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			rules, err := st.ListAlertRules()
			if err != nil {
				return err
			}
			formatAlertRules(cmd.OutOrStdout(), rules)
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an alert rule by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			st, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			if err := st.RemoveAlertRule(id); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "alert rule #%d removed\n", id)
			return nil
		},
	}

	cmd.AddCommand(set, list, remove)
	cmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	return cmd
}
