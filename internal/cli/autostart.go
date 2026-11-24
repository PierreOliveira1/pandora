package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

func defaultAutostart(paths config.Paths) (daemon.Autostart, error) {
	exe, err := os.Executable()
	if err != nil {
		return daemon.Autostart{}, fmt.Errorf("finding the pandora executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return daemon.Autostart{}, fmt.Errorf("resolving the pandora executable: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return daemon.Autostart{}, fmt.Errorf("finding the home directory: %w", err)
	}
	return daemon.Autostart{
		Exe:        resolved,
		Home:       home,
		Env:        daemon.AutostartEnv(os.Getenv, os.Environ()),
		LogPath:    paths.DaemonLog(),
		UID:        currentUID(),
		ConfigHome: os.Getenv("XDG_CONFIG_HOME"),
		Runner:     tunnel.ExecRunner{},
	}, nil
}

func newAutostart(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autostart",
		Short: "Start the Pandora daemon at login",
	}
	cmd.AddCommand(newAutostartOn(d), newAutostartOff(d), newAutostartStatus(d))
	return cmd
}

func newAutostartOn(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Start Pandora at login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			autostart, err := d.autostartConfig()
			if err != nil {
				return err
			}
			if err := autostart.Enable(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(d.stdout, "Pandora now starts at login.")
			return nil
		},
	}
}

func newAutostartOff(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Do not start Pandora at login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			autostart, err := d.autostartConfig()
			if err != nil {
				return err
			}
			if err := autostart.Disable(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(d.stdout, "Pandora no longer starts at login.")
			return nil
		},
	}
}

func newAutostartStatus(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether Pandora starts at login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			autostart, err := d.autostartConfig()
			if err != nil {
				return err
			}
			enabled, err := autostart.Enabled(cmd.Context())
			if err != nil {
				return err
			}
			if enabled {
				fmt.Fprintln(d.stdout, "on")
			} else {
				fmt.Fprintln(d.stdout, "off")
			}
			return nil
		},
	}
}

func (d deps) autostartConfig() (daemon.Autostart, error) {
	if d.autostart == nil {
		return daemon.Autostart{}, errors.New("autostart is not configured")
	}
	return d.autostart()
}

func (d deps) suggestAutostart(ctx context.Context) {
	if d.autostart == nil {
		return
	}
	autostart, err := d.autostart()
	if err != nil {
		return
	}
	enabled, err := autostart.Enabled(ctx)
	if err != nil || enabled {
		return
	}
	fmt.Fprintln(d.stdout, "To keep receiving events after a restart: pandora autostart on")
}
