package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/upgrade"
)

func newUpgrade(d deps) *cobra.Command {
	var check bool
	var targetVersion string

	cmd := &cobra.Command{
		Use:   "upgrade [--check] [--version X.Y.Z]",
		Short: "Upgrade pandora to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			current := appVersion()
			requested := strings.TrimPrefix(strings.TrimSpace(targetVersion), "v")
			upgrader := upgrade.Upgrader{
				Client:  &http.Client{Timeout: 5 * time.Minute},
				BaseURL: "https://github.com",
				Repo:    "PierreOliveira1/pandora",
				GOOS:    runtime.GOOS,
				GOARCH:  runtime.GOARCH,
			}

			target := requested
			if target == "" {
				if !check && current == "dev" {
					return errors.New("this pandora was built from source; reinstall with go install github.com/PierreOliveira1/pandora/cmd/pandora@latest or the install script, or pass --version")
				}
				latest, err := upgrader.Latest(cmd.Context())
				if err != nil {
					return err
				}
				target = latest
			}
			if check {
				if upgrade.Newer(target, current) {
					fmt.Fprintf(d.stdout, "pandora %s is available (current %s). Run pandora upgrade.\n", target, current)
				} else {
					fmt.Fprintf(d.stdout, "pandora %s is up to date.\n", current)
				}
				return nil
			}
			if requested == "" && !upgrade.Newer(target, current) {
				fmt.Fprintf(d.stdout, "pandora %s is up to date.\n", current)
				return nil
			}

			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("finding the pandora executable: %w", err)
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return fmt.Errorf("resolving the pandora executable: %w", err)
			}
			daemonRunning := false
			if _, err := daemon.Dial(cmd.Context(), d.paths.DaemonSocket()); err == nil {
				daemonRunning = true
			}
			if err := upgrader.Install(cmd.Context(), target, exe); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Upgraded pandora %s -> %s.\n", current, target)
			if err := runUpgradeStep(cmd.Context(), exe, d.stdout, d.stderr, "sync"); err != nil {
				fmt.Fprintf(d.stderr, "the upgrade worked, but %s sync failed: %v; run it again\n", exe, err)
			}
			if daemonRunning {
				if err := runUpgradeStep(cmd.Context(), exe, d.stdout, d.stderr, "daemon", "start"); err != nil {
					fmt.Fprintf(d.stderr, "the upgrade worked, but %s daemon start failed: %v; run it again\n", exe, err)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "only check for a newer version")
	cmd.Flags().StringVar(&targetVersion, "version", "", "install this version")
	return cmd
}

func runUpgradeStep(ctx context.Context, exe string, stdout, stderr io.Writer, args ...string) error {
	command := exec.CommandContext(ctx, exe, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
