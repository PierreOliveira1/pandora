package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctor(d deps) *cobra.Command {
	var offline, asJSON bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configuration and connections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := d.core.Doctor(cmd.Context(), offline)

			if asJSON {
				if err := writeJSON(d.stdout, checks); err != nil {
					return err
				}
			} else {
				for _, check := range checks {
					symbol := "✓"
					if !check.OK {
						symbol = "✗"
					}
					if check.Info != "" {
						fmt.Fprintf(d.stdout, "%s %s  %s\n", symbol, check.Name, check.Info)
					} else {
						fmt.Fprintf(d.stdout, "%s %s\n", symbol, check.Name)
					}
					if !check.OK && check.Fix != "" {
						fmt.Fprintf(d.stdout, "    fix: %s\n", check.Fix)
					}
				}
			}

			for _, check := range checks {
				if !check.OK {
					return &silentExitError{code: 1}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&offline, "offline", false, "skip live account checks")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}
