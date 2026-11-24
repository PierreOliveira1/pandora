package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/daemon"
)

type daemonBackend struct {
	client *daemon.Client
}

func (b daemonBackend) Tools(ctx context.Context, service string, refresh bool) (*core.Catalog, error) {
	return b.client.Tools(ctx, service, refresh)
}

func (b daemonBackend) Describe(ctx context.Context, service, tool string) (*mcp.Tool, error) {
	return b.client.Describe(ctx, service, tool)
}

func (b daemonBackend) Search(ctx context.Context, query, service string, limit int) ([]core.SearchHit, int, error) {
	return b.client.Search(ctx, query, service, limit)
}

func (b daemonBackend) Call(ctx context.Context, ref, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	return b.client.Call(ctx, ref, tool, args)
}

func (b daemonBackend) StatusAll(ctx context.Context) []core.StatusInfo {
	infos, err := b.client.Status(ctx, "")
	if err != nil {
		return []core.StatusInfo{{Ref: "daemon", Error: err.Error()}}
	}
	return infos
}

func (b daemonBackend) Status(ctx context.Context, ref string) core.StatusInfo {
	infos, err := b.client.Status(ctx, ref)
	if err != nil {
		return core.StatusInfo{Ref: ref, Error: err.Error()}
	}
	if len(infos) == 0 {
		return core.StatusInfo{Ref: ref}
	}
	return infos[0]
}

func newDaemon(d deps) *cobra.Command {
	var idle time.Duration

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the shared connection daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if idle <= 0 {
				return fmt.Errorf("--idle must be positive")
			}
			server := daemon.NewServer(d.core, buildID(), idle)
			err := server.Serve(cmd.Context(), d.paths.DaemonSocket())
			if errors.Is(err, daemon.ErrAlreadyRunning) {
				fmt.Fprintln(d.stderr, "pandora daemon is already running")
				return nil
			}
			return err
		},
	}

	cmd.Flags().DurationVar(&idle, "idle", 15*time.Minute, "stop after this much inactivity")
	cmd.AddCommand(newDaemonStatus(d), newDaemonStop(d), newDaemonStart(d))
	return cmd
}

func newDaemonStatus(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.Dial(cmd.Context(), d.paths.DaemonSocket())
			if err != nil {
				fmt.Fprintln(d.stdout, "not running")
				return nil
			}
			health, err := client.Health(cmd.Context())
			if err != nil {
				fmt.Fprintln(d.stdout, "not running")
				return nil
			}
			fmt.Fprintf(d.stdout, "pid: %d\nversion: %s\nconnections: %d\nuptime: %s\n",
				health.PID,
				health.Version,
				health.Connections,
				time.Since(health.StartedAt).Round(time.Second),
			)
			return nil
		},
	}
}

func newDaemonStop(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.Dial(cmd.Context(), d.paths.DaemonSocket())
			if err != nil {
				fmt.Fprintln(d.stdout, "not running")
				return nil
			}
			if err := client.Shutdown(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(d.stdout, "stopped")
			return nil
		},
	}
}

func newDaemonStart(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("finding the pandora executable: %w", err)
			}
			client, err := daemon.Ensure(cmd.Context(), d.paths, exe, buildID())
			if err != nil {
				return err
			}
			health, err := client.Health(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "pid: %d\n", health.PID)
			return nil
		},
	}
}
