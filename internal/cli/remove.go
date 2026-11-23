package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newRemove(d deps) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:         "remove <service/account>",
		Short:       "Remove an account or a service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}

			if ref.Account != "" {
				if err := d.registry.RemoveAccount(ref); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Removed %s\n", ref.String())
				return nil
			}

			accounts, emptyServices, err := d.registry.List()
			if err != nil {
				return err
			}
			count := 0
			exists := false
			for _, account := range accounts {
				if account.Service == ref.Service {
					count++
					exists = true
				}
			}
			for _, service := range emptyServices {
				if service == ref.Service {
					exists = true
				}
			}
			if !exists {
				return fmt.Errorf("unknown service %q", ref.Service)
			}

			if count > 0 && !yes {
				if !isTerminal(d.stdin) {
					return fmt.Errorf("use --yes to remove service %s and its %d accounts", ref.Service, count)
				}
				fmt.Fprintf(d.stdout, "Remove service %s and its %d accounts? [y/N] ", ref.Service, count)
				confirmed, err := readConfirmation(d.stdin)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(d.stdout, "Cancelled.")
					return nil
				}
			}

			if err := d.registry.RemoveService(ref.Service); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Removed service %s\n", ref.Service)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}
