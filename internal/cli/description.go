package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newSetDescription(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "set-description <service|service/account> <text>",
		Short:       "Set the description of a service or account",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if err := d.registry.SetDescription(ref, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Updated description of %s\n", ref.String())
			return nil
		},
	}
}
