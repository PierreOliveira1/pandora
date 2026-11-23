package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newAdd(d deps) *cobra.Command {
	var (
		account      string
		rawURL       string
		openapi      string
		kindFlag     string
		envFlags     []string
		secretFlags  []string
		headerFlags  []string
		oauth        bool
		clientID     string
		clientSecret string
		scopes       []string
		description  string
		serviceDesc  string
		noLogin      bool
	)

	cmd := &cobra.Command{
		Use:         "add <service> --as <account> [flags] [-- command args...]",
		Short:       "Add a service account",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("service is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if account == "" {
				return errors.New("--as is required")
			}

			service := args[0]
			var command []string
			if len(args) > 1 {
				if cmd.ArgsLenAtDash() != 1 {
					return errors.New("pass command arguments after --")
				}
				command = args[1:]
			}

			kind, err := resolveKind(kindFlag, rawURL, openapi, command)
			if err != nil {
				return err
			}
			env, err := parseEnv(envFlags)
			if err != nil {
				return err
			}
			secretEnv, err := parseSecretEnv(secretFlags, d.stdin, d.stderr)
			if err != nil {
				return err
			}
			headers, err := parseHeaders(headerFlags, d.stdin, d.stderr)
			if err != nil {
				return err
			}

			in := registry.AddInput{
				Service:            service,
				Account:            account,
				Kind:               kind,
				URL:                rawURL,
				Command:            command,
				Spec:               openapi,
				OAuthClientID:      clientID,
				OAuthClientSecret:  clientSecret,
				OAuthScopes:        scopes,
				Description:        description,
				ServiceDescription: serviceDesc,
				Env:                env,
				SecretEnv:          secretEnv,
				SecretHeaders:      headers,
			}
			if oauth {
				in.Auth = config.AuthOAuth
			}
			if err := d.registry.Add(in); err != nil {
				return err
			}

			accounts, _, err := d.registry.List()
			if err != nil {
				return err
			}
			var added *registry.AccountInfo
			for i := range accounts {
				if accounts[i].Service == service && accounts[i].Account == account {
					added = &accounts[i]
					break
				}
			}
			if added == nil {
				return fmt.Errorf("account %q is missing after add", service+"/"+account)
			}

			suffix := ""
			if added.Default {
				suffix = ", default"
			}
			fmt.Fprintf(d.stdout, "Added %s/%s (%s%s)\n", service, account, added.Kind, suffix)

			if added.Kind != config.KindMCPStdio && added.Kind != config.KindMCPHTTP {
				return nil
			}

			connectCtx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			ref := service + "/" + account
			status, err := d.core.DetectAuth(connectCtx, ref)
			if err != nil {
				fmt.Fprintf(d.stdout, "Saved, but could not connect yet: %v\n", err)
				return nil
			}
			if status.NeedsLogin {
				if isTerminal(d.stdin) && !noLogin {
					fmt.Fprintln(d.stdout, "This service needs a login.")
					if err := d.core.Login(cmd.Context(), ref); err != nil {
						fmt.Fprintf(d.stdout, "Saved, but could not connect yet: %v\n", err)
						return nil
					}
					toolsCtx, toolsCancel := context.WithTimeout(cmd.Context(), 20*time.Second)
					defer toolsCancel()
					catalog, err := d.core.Tools(toolsCtx, service, false)
					if err != nil {
						fmt.Fprintln(d.stdout, "Logged in.")
						return nil
					}
					fmt.Fprintf(d.stdout, "Connected: %d tools available\n", len(catalog.Tools))
					return nil
				}
				fmt.Fprintf(d.stdout, "Login required: run pandora auth %s\n", ref)
				return nil
			}
			if !status.OK {
				message := status.Error
				if message == "" {
					message = "connection failed"
				}
				fmt.Fprintf(d.stdout, "Saved, but could not connect yet: %s\n", message)
				return nil
			}

			catalog, err := d.core.Tools(connectCtx, service, true)
			if err != nil {
				fmt.Fprintf(d.stdout, "Connected: %d tools available\n", status.Tools)
				return nil
			}
			fmt.Fprintf(d.stdout, "Connected: %d tools available\n", len(catalog.Tools))
			return nil
		},
	}

	cmd.Flags().StringVar(&account, "as", "", "account alias")
	cmd.Flags().StringVar(&rawURL, "url", "", "MCP server URL")
	cmd.Flags().StringVar(&openapi, "openapi", "", "path or URL of an OpenAPI spec")
	cmd.Flags().StringVar(&kindFlag, "kind", "", `service kind (only "command")`)
	cmd.Flags().StringArrayVar(&envFlags, "env", nil, "environment variable KEY=VALUE (repeatable)")
	cmd.Flags().StringArrayVar(&secretFlags, "secret-env", nil, "secret environment variable NAME or NAME=VALUE (repeatable)")
	cmd.Flags().StringArrayVar(&headerFlags, "header", nil, "secret header Name or Name: value (repeatable)")
	cmd.Flags().BoolVar(&oauth, "oauth", false, "use OAuth")
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "OAuth client secret")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "OAuth scope (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "account description")
	cmd.Flags().StringVar(&serviceDesc, "service-description", "", "service description (only when creating the service)")
	cmd.Flags().BoolVar(&noLogin, "no-login", false, "do not start the OAuth login")

	return cmd
}

func resolveKind(kindFlag, rawURL, openapi string, command []string) (config.Kind, error) {
	if kindFlag != "" && kindFlag != string(config.KindCommand) {
		return "", fmt.Errorf("unsupported kind %q: only %q is accepted", kindFlag, config.KindCommand)
	}

	var kind config.Kind
	sources := 0
	if len(command) > 0 {
		sources++
		kind = config.KindMCPStdio
	}
	if rawURL != "" {
		sources++
		kind = config.KindMCPHTTP
	}
	if openapi != "" {
		sources++
		kind = config.KindOpenAPI
	}
	if kindFlag != "" {
		sources++
		kind = config.KindCommand
	}
	if sources > 1 {
		return "", errors.New("use only one source: a command after --, --url, --openapi or --kind command")
	}
	return kind, nil
}

func parseEnv(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	env := make(map[string]string, len(values))
	for _, raw := range values {
		name, value, ok := strings.Cut(raw, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid env %q: use NAME=VALUE", raw)
		}
		env[name] = value
	}
	return env, nil
}

func parseSecretEnv(values []string, stdin io.Reader, stderr io.Writer) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	secretEnv := make(map[string]string, len(values))
	for _, raw := range values {
		name, value, ok := strings.Cut(raw, "=")
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("invalid secret env %q: use NAME or NAME=VALUE", raw)
		}
		if !ok {
			var err error
			value, err = promptSecret(stdin, stderr, name)
			if err != nil {
				return nil, err
			}
		}
		secretEnv[name] = value
	}
	return secretEnv, nil
}

func parseHeaders(values []string, stdin io.Reader, stderr io.Writer) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	headers := make(map[string]string, len(values))
	for _, raw := range values {
		name, value, hasValue := strings.Cut(raw, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("invalid header %q: use Name or Name: value", raw)
		}
		if !hasValue {
			var err error
			value, err = promptSecret(stdin, stderr, name)
			if err != nil {
				return nil, err
			}
		}
		headers[http.CanonicalHeaderKey(name)] = strings.TrimSpace(value)
	}
	return headers, nil
}
