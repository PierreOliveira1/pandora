package cli

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newDescribe(d deps) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "describe <service> <tool>",
		Short: "Show a tool's description and input schema",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			backend := d.backend(cmd.Context())
			tool, err := backend.Describe(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(d.stdout, tool)
			}

			fmt.Fprintf(d.stdout, "%s%s\n", tool.Name, toolSuffix(tool))
			if tool.Description != "" {
				fmt.Fprintln(d.stdout, tool.Description)
			}
			schema, err := json.MarshalIndent(tool.InputSchema, "", "  ")
			if err != nil {
				return fmt.Errorf("encoding input schema: %w", err)
			}
			fmt.Fprintln(d.stdout, "Input schema:")
			fmt.Fprintln(d.stdout, string(schema))
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func toolSuffix(tool *mcp.Tool) string {
	if tool.Annotations == nil {
		return ""
	}
	if tool.Annotations.ReadOnlyHint {
		return " (read-only)"
	}
	if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
		return " (destructive)"
	}
	return ""
}
