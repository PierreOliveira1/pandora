package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSync(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Regenerate the Pandora skill in installed AI clients",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := d.core.SyncSkills(cmd.Context())
			for _, result := range results {
				status := "unchanged"
				if result.Changed {
					status = "updated"
				}
				fmt.Fprintf(d.stdout, "%s: %s\n", result.Client, status)
			}
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Fprintln(d.stdout, "No AI clients have the skill. Run pandora install.")
			}
			return nil
		},
	}
}
