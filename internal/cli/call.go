package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newCall(d deps) *cobra.Command {
	var argFlags []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "call <ref> <tool> [json-args|-]",
		Short: "Call a tool on an account",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			arguments := map[string]any{}
			if len(args) == 3 {
				parsed, err := parseArguments(args[2], d.stdin)
				if err != nil {
					return err
				}
				arguments = parsed
			}
			if err := applyArgumentFlags(arguments, argFlags); err != nil {
				return err
			}

			backend := d.backend(cmd.Context())
			result, err := backend.Call(cmd.Context(), args[0], args[1], arguments)
			if err != nil {
				return err
			}
			if result.IsError {
				if err := writeCallResult(d.stderr, result); err != nil {
					return err
				}
				if asJSON {
					if err := writeJSON(d.stdout, result); err != nil {
						return err
					}
				}
				return &silentExitError{code: 1}
			}
			if asJSON {
				return writeJSON(d.stdout, result)
			}
			return writeCallResult(d.stdout, result)
		},
	}

	cmd.Flags().StringArrayVar(&argFlags, "arg", nil, "tool argument KEY=VALUE (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func parseArguments(spec string, stdin io.Reader) (map[string]any, error) {
	data := []byte(spec)
	if spec == "-" {
		var err error
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("reading arguments from stdin: %w", err)
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("json arguments are empty")
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("invalid json arguments: %w", err)
	}
	arguments, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("json arguments must be an object")
	}
	return arguments, nil
}

func applyArgumentFlags(arguments map[string]any, flags []string) error {
	for _, flag := range flags {
		key, value, ok := strings.Cut(flag, "=")
		if !ok || key == "" {
			return fmt.Errorf("invalid --arg %q: use KEY=VALUE", flag)
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			decoded = value
		}
		arguments[key] = decoded
	}
	return nil
}

func writeCallResult(w io.Writer, result *mcp.CallToolResult) error {
	hasText := false
	for _, content := range result.Content {
		switch value := content.(type) {
		case *mcp.TextContent:
			hasText = true
			fmt.Fprintln(w, value.Text)
		case *mcp.ImageContent:
			fmt.Fprintf(w, "[image %s]\n", value.MIMEType)
		case *mcp.AudioContent:
			fmt.Fprintf(w, "[audio %s]\n", value.MIMEType)
		case *mcp.ResourceLink:
			fmt.Fprintf(w, "[resource %s]\n", value.URI)
		case *mcp.EmbeddedResource:
			uri := ""
			if value.Resource != nil {
				uri = value.Resource.URI
			}
			fmt.Fprintf(w, "[resource %s]\n", uri)
		}
	}

	if !hasText && result.StructuredContent != nil {
		data, err := json.MarshalIndent(result.StructuredContent, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding structured content: %w", err)
		}
		fmt.Fprintln(w, string(data))
	}
	return nil
}
