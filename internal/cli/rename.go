package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newRename(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "rename <service/account> <new-account>",
		Short:       "Rename an account or a service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}

			if ref.Account != "" {
				if err := d.registry.RenameAccount(ref, args[1]); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Renamed %s to %s/%s\n", ref.String(), ref.Service, args[1])
				return nil
			}

			if err := d.registry.RenameService(ref.Service, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Renamed service %s to %s\n", ref.Service, args[1])
			return nil
		},
	}
}
