package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

type listOutput struct {
	Accounts      []registry.AccountInfo `json:"accounts"`
	EmptyServices []string               `json:"empty_services"`
}

func newList(d deps) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List services and accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			accounts, emptyServices, err := d.registry.List()
			if err != nil {
				return err
			}

			if asJSON {
				encoder := json.NewEncoder(d.stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(listOutput{Accounts: accounts, EmptyServices: emptyServices})
			}

			if len(accounts) == 0 && len(emptyServices) == 0 {
				fmt.Fprintln(d.stdout, "No accounts yet. Connect a service with pandora add (pandora add --help shows MCP servers, OpenAPI APIs and your own commands).")
				return nil
			}

			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "ACCOUNT\tKIND\tAUTH\tDEFAULT")
			for _, account := range accounts {
				mark := ""
				if account.Default {
					mark = "*"
				}
				fmt.Fprintf(writer, "%s/%s\t%s\t%s\t%s\n", account.Service, account.Account, account.Kind, account.Auth, mark)
			}
			for _, service := range emptyServices {
				fmt.Fprintf(writer, "%s/\t(no accounts)\n", service)
			}
			return writer.Flush()
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}
