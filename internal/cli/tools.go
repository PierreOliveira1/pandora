package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newTools(d deps) *cobra.Command {
	var refresh, asJSON bool

	cmd := &cobra.Command{
		Use:         "tools [service]",
		Short:       "List the tools of connected services",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backend := d.backend(cmd.Context())
			explicit := len(args) == 1
			services := args
			if !explicit {
				var err error
				services, err = servicesWithAccounts(d)
				if err != nil {
					return err
				}
			}

			if asJSON {
				output := make(map[string][]*mcp.Tool, len(services))
				for _, service := range services {
					catalog, err := backend.Tools(cmd.Context(), service, refresh)
					if err != nil {
						if explicit {
							return err
						}
						continue
					}
					output[service] = catalog.Tools
				}
				return writeJSON(d.stdout, output)
			}

			if len(services) == 0 {
				fmt.Fprintln(d.stdout, "No accounts yet. Add one with: pandora add <service> --as <account> --url <mcp-url>")
				return nil
			}

			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			for _, service := range services {
				catalog, err := backend.Tools(cmd.Context(), service, refresh)
				if err != nil {
					if explicit {
						return err
					}
					fmt.Fprintf(writer, "%s\n  error: %v\n", service, err)
					continue
				}
				fmt.Fprintf(writer, "%s (%d tools)\n", service, len(catalog.Tools))
				for _, tool := range catalog.Tools {
					fmt.Fprintf(writer, "  %s\t%s\n", tool.Name, firstLine(tool.Description))
				}
			}
			return writer.Flush()
		},
	}

	cmd.Flags().BoolVar(&refresh, "refresh", false, "fetch the tools again")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func servicesWithAccounts(d deps) ([]string, error) {
	accounts, _, err := d.registry.List()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	services := make([]string, 0)
	for _, account := range accounts {
		if seen[account.Service] {
			continue
		}
		seen[account.Service] = true
		services = append(services, account.Service)
	}
	return services, nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
