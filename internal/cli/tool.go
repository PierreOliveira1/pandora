package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newTool(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "Manage tools of command services",
	}
	cmd.AddCommand(newToolAdd(d), newToolRemove(d))
	return cmd
}

func newToolAdd(d deps) *cobra.Command {
	var (
		description string
		params      []string
		readOnly    bool
		timeout     int
	)
	cmd := &cobra.Command{
		Use:         "add <service> <name> [flags] -- <command and args>",
		Short:       "Add a tool to a command service",
		Long:        "Add a tool to a command service. The command runs directly, without a shell: pipes, redirections and $VARS are not expanded, so call a script for more than one step.\n\nEach {{name}} in the command is replaced by the argument with that name, as a whole argument or inside one (--title={{title}}). A command argument that holds a missing optional parameter is dropped. An array used as a whole argument becomes one argument per item.\n\nThe account's --env and --secret-env values reach the program as environment variables. The program's stdout is the result; a non-zero exit is an error shown with its stderr.",
		Annotations: map[string]string{syncAnnotation: "true"},
		Example: `  pandora tool add git log --readonly --description "Show recent commits" \
    --param repo:string:required:"Repository path" --param n:integer \
    -- git -C {{repo}} log --max-count={{n}} --oneline

  pandora tool add instagram post-reel --timeout 600 --description "Publish a video as a Reel" \
    --param video_url:string:required:"Public HTTPS URL of the MP4" --param caption:string \
    -- /Users/me/scripts/post-reel.sh {{video_url}} {{caption}}`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 2 {
				return errors.New("pass the command and its arguments after --")
			}
			service := args[0]
			name := args[1]
			parsed, err := parseCommandParams(params)
			if err != nil {
				return err
			}
			err = d.registry.AddCommandTool(registry.CommandToolInput{
				Service:     service,
				Name:        name,
				Description: description,
				Command:     args[2:],
				Params:      parsed,
				ReadOnly:    readOnly,
				TimeoutSec:  timeout,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Added tool %s/%s\n", service, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "tool description")
	cmd.Flags().StringArrayVar(&params, "param", nil, "parameter as name:type[:required][:description] (repeatable)")
	cmd.Flags().BoolVar(&readOnly, "readonly", false, "mark the tool as read-only")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "timeout in seconds (default 60)")
	return cmd
}

func newToolRemove(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "remove <service> <name>",
		Short:       "Remove a tool from a command service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := d.registry.RemoveCommandTool(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Removed tool %s/%s\n", args[0], args[1])
			return nil
		},
	}
}

func parseCommandParams(values []string) (map[string]config.CommandParam, error) {
	params := make(map[string]config.CommandParam, len(values))
	for _, raw := range values {
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid --param %q: use name:type[:required][:description]", raw)
		}
		parameter := config.CommandParam{Type: parts[1]}
		if len(parts) >= 3 {
			if parts[2] == "required" {
				parameter.Required = true
				if len(parts) == 4 {
					parameter.Description = parts[3]
				}
			} else {
				parameter.Description = parts[2]
				if len(parts) == 4 {
					parameter.Description += ":" + parts[3]
				}
			}
		}
		params[parts[0]] = parameter
	}
	return params, nil
}
