package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPaths(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Print the config, cache and state directories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(d.stdout, "config: %s\ncache: %s\nstate: %s\n", d.paths.ConfigDir, d.paths.CacheDir, d.paths.StateDir)
			return nil
		},
	}
}
