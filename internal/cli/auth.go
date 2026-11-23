package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newAuth(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "auth <service|service/account>",
		Short:       "Log in to an OAuth account",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if err := d.core.Login(cmd.Context(), ref.String()); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Logged in to %s.\n", ref.String())

			catalogCtx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			catalog, err := d.core.Tools(catalogCtx, ref.Service, false)
			if err == nil {
				fmt.Fprintf(d.stdout, "%d tools available.\n", len(catalog.Tools))
			}
			return nil
		},
	}
}

func newLogout(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "logout <service|service/account>",
		Short:       "Log out of an OAuth account",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if err := d.core.Logout(cmd.Context(), ref.String()); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Logged out of %s.\n", ref.String())
			return nil
		},
	}
}
