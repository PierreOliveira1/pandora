package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func newExpose(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "expose <service> [compact|direct]",
		Short: "Show or change how a service is exposed in MCP clients",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := args[0]
			if len(args) == 1 {
				overview, err := d.core.Overview(cmd.Context())
				if err != nil {
					return err
				}
				for _, view := range overview.Services {
					if view.Name == service {
						fmt.Fprintf(d.stdout, "%s uses %s mode in MCP clients.\n", service, view.Expose)
						return nil
					}
				}
				return fmt.Errorf("unknown service %q", service)
			}
			mode := config.ExposeMode(args[1])
			if err := d.registry.SetExpose(service, mode); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "%s now uses %s mode in MCP clients (restart them to see the change)\n", service, mode)
			return nil
		},
	}
}
