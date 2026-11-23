package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newDefault(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "default <service/account>",
		Short:       "Set the default account of a service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if ref.Account == "" {
				return fmt.Errorf("account is required: use %s/<account>", ref.Service)
			}
			if err := d.registry.SetDefault(ref); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "%s is now the default account\n", ref.String())
			return nil
		},
	}
}
