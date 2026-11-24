package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newEnv(d deps) *cobra.Command {
	var remove string

	cmd := &cobra.Command{
		Use:   "env <service>/<account> [NAME=VALUE...] [--remove NAME]",
		Short: "Manage the account's non-secret values",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if remove != "" {
				if len(args) != 1 {
					return errors.New("use pandora env <service>/<account> --remove NAME")
				}
				if err := d.registry.UnsetAccountEnv(ref, remove); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Removed %s from %s.\n", remove, ref)
				return nil
			}
			if len(args) == 1 {
				info, err := d.registry.GetAccountEnv(ref)
				if err != nil {
					return err
				}
				keys := sortedStringKeys(info.Env)
				for _, name := range keys {
					fmt.Fprintf(d.stdout, "%s=%s\n", name, info.Env[name])
				}
				secrets := append([]string(nil), info.SecretEnv...)
				sort.Strings(secrets)
				for _, name := range secrets {
					fmt.Fprintf(d.stdout, "%s (secret)\n", name)
				}
				if len(keys) == 0 && len(secrets) == 0 {
					fmt.Fprintf(d.stdout, "No values for %s.\n", ref)
				}
				return nil
			}
			for _, raw := range args[1:] {
				name, value, ok := strings.Cut(raw, "=")
				name = strings.TrimSpace(name)
				if !ok || name == "" {
					return fmt.Errorf("invalid env %q: use NAME=VALUE", raw)
				}
				if err := d.registry.SetAccountEnv(ref, name, value); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Set %s for %s.\n", name, ref)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&remove, "remove", "", "remove NAME")
	return cmd
}
