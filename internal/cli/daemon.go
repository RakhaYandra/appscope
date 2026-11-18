package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/RakhaYandra/appscope/internal/daemon"
	"github.com/RakhaYandra/appscope/internal/ipc"
	"github.com/RakhaYandra/appscope/internal/store"
)

func newDaemonCmd(socketPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Control the AppScope background collector",
	}

	var interval time.Duration
	var dbPath string
	run := &cobra.Command{
		Use:   "run",
		Short: "Run the collector in the foreground (used by systemd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd, *socketPath, interval, dbPath)
		},
	}
	run.Flags().DurationVar(&interval, "interval", time.Second, "sampling interval")
	run.Flags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")

	cmd.AddCommand(run)
	for _, name := range []string{"start", "stop", "restart", "status"} {
		name := name
		sub := &cobra.Command{
			Use:   name,
			Short: "systemctl --user " + name + " appscoped.service",
			RunE: func(cmd *cobra.Command, args []string) error {
				c := exec.Command("systemctl", "--user", name, "appscoped.service")
				c.Stdout = cmd.OutOrStdout()
				c.Stderr = cmd.ErrOrStderr()
				return c.Run()
			},
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func defaultDBPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "appscope.db"
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "appscope", "appscope.db")
}

func runDaemon(cmd *cobra.Command, socketPath string, interval time.Duration, dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	d := daemon.New(st)
	d.Interval = interval

	srv, err := ipc.NewServer(socketPath, d)
	if err != nil {
		return err
	}
	go func() { _ = srv.Serve() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	cmd.PrintErrf("appscoped: sampling every %s, db=%s, socket=%s\n", interval, dbPath, socketPath)

	select {
	case err := <-errCh:
		_ = srv.Close()
		return err
	case <-ctx.Done():
		_ = srv.Close()
		<-errCh
		return nil
	}
}
