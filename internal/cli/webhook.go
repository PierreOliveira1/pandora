package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newWebhook(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage incoming webhooks",
	}
	cmd.AddCommand(
		newWebhookAdd(d),
		newWebhookList(d),
		newWebhookShow(d),
		newWebhookRemove(d),
		newWebhookTest(d),
	)
	return cmd
}

func newWebhookAdd(d deps) *cobra.Command {
	var (
		style           string
		challenge       string
		signatureHeader string
		signaturePrefix string
		encoding        string
		tokenHeader     string
		tokenQuery      string
		topicHeader     string
		topicFields     []string
		idHeader        string
		insecure        bool
	)

	cmd := &cobra.Command{
		Use:         "add <service>[/<account>] --style hmac|token|none [flags]",
		Short:       "Receive webhook deliveries for an account",
		Long:        "Add a webhook to an account. Pandora verifies each delivery, stores it in the local event queue and prints the callback URL to paste in the service's settings.\n\nFor hmac, the signing secret is asked in the terminal without being shown, or read from stdin. For token, Pandora generates the token. For none, add --insecure: deliveries are accepted from anyone.\n\nThe verify token is only used for the challenge handshake. The signing secret and the token style token are credentials: share them only with the service.",
		Annotations: map[string]string{syncAnnotation: "true"},
		Example: `  # Meta style: hub challenge and a topic from the event body
  pandora webhook add instagram/store --style hmac --challenge hub \
    --topic-field entry.0.changes.0.field --topic-field entry.0.messaging

  # GitHub style: a topic and a delivery id in headers
  pandora webhook add github/work --style hmac --topic-header X-GitHub-Event \
    --id-header X-GitHub-Delivery

  # A shared token in a query parameter
  pandora webhook add store/orders --style token --token-query token`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if style == "" {
				return errors.New("--style is required: use hmac, token or none")
			}
			in := registry.WebhookInput{
				Ref:             ref,
				Style:           style,
				Challenge:       challenge,
				SignatureHeader: signatureHeader,
				SignaturePrefix: signaturePrefix,
				Encoding:        encoding,
				TokenHeader:     tokenHeader,
				TokenQuery:      tokenQuery,
				TopicHeader:     topicHeader,
				TopicFields:     topicFields,
				IDHeader:        idHeader,
				Insecure:        insecure,
			}
			if _, err := d.registry.CheckWebhook(in); err != nil {
				return err
			}
			if strings.EqualFold(style, "hmac") {
				in.Secret, err = newSecretReader(d.stdin, d.stderr).Read("WEBHOOK_SECRET")
				if err != nil {
					return err
				}
			}

			added, err := d.registry.AddWebhook(in)
			if err != nil {
				return err
			}
			if _, err := d.core.EnsureWebhookPort(cmd.Context()); err != nil {
				return err
			}
			d.reloadWebhooks(cmd.Context(), true)

			infos, err := d.core.Webhooks(cmd.Context())
			if err != nil {
				return err
			}
			info, ok := findWebhookInfo(infos, added.Ref.String())
			if !ok {
				return fmt.Errorf("webhook added to %s, but its URL is missing from the config", added.Ref)
			}

			fmt.Fprintf(d.stdout, "Webhook added to %s.\n", added.Ref)
			if info.URL != "" {
				fmt.Fprintf(d.stdout, "Callback URL: %s\n", info.URL)
			} else {
				fmt.Fprintln(d.stdout, "Callback URL: none yet: run pandora tunnel use tailscale")
				if info.LocalURL != "" {
					fmt.Fprintf(d.stdout, "Local URL: %s\n", info.LocalURL)
				}
			}
			if added.VerifyToken != "" {
				fmt.Fprintf(d.stdout, "Verify token: %s\n", added.VerifyToken)
			}
			if added.Webhook.Style == "token" {
				if outputIsTerminal(d.stdout) {
					fmt.Fprintf(d.stdout, "Token: %s\n", added.GeneratedToken)
				} else {
					fmt.Fprintf(d.stdout, "Token: run pandora webhook show %s --token in your terminal to see it\n", added.Ref)
				}
			}
			fmt.Fprintf(d.stdout, "Paste them in the service's webhook settings, then check with: pandora webhook test %s\n", added.Ref)
			fmt.Fprintf(d.stdout, "Events: pandora events %s\n", added.Ref)
			d.suggestAutostart(cmd.Context())
			return nil
		},
	}

	cmd.Flags().StringVar(&style, "style", "", "verification style: hmac, token or none")
	cmd.Flags().StringVar(&challenge, "challenge", "", "answer the hub challenge with hub")
	cmd.Flags().StringVar(&signatureHeader, "signature-header", "", "signature header name (default X-Hub-Signature-256)")
	cmd.Flags().StringVar(&signaturePrefix, "signature-prefix", "", "signature value prefix (default sha256=)")
	cmd.Flags().StringVar(&encoding, "encoding", "", "signature encoding: hex or base64 (default hex)")
	cmd.Flags().StringVar(&tokenHeader, "token-header", "", "header that carries the token")
	cmd.Flags().StringVar(&tokenQuery, "token-query", "", "query parameter that carries the token")
	cmd.Flags().StringVar(&topicHeader, "topic-header", "", "header that carries the event topic")
	cmd.Flags().StringArrayVar(&topicFields, "topic-field", nil, "JSON path that carries the event topic (repeatable)")
	cmd.Flags().StringVar(&idHeader, "id-header", "", "header that carries the delivery id")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "accept unsigned deliveries with --style none")
	return cmd
}

func newWebhookList(d deps) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List webhooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			infos, err := d.core.Webhooks(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				if infos == nil {
					infos = []core.WebhookInfo{}
				}
				return writeJSON(d.stdout, infos)
			}
			if len(infos) == 0 {
				fmt.Fprintln(d.stdout, "No webhooks yet. Add one with pandora webhook add <service>/<account> --style hmac")
				return nil
			}
			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "REF\tSTYLE\tURL\tLAST EVENT")
			for _, info := range infos {
				url := info.URL
				if url == "" {
					url = info.LocalURL
				}
				if url == "" {
					url = "none"
				}
				last := "never"
				if info.LastEventAt != nil {
					last = info.LastEventAt.Local().Format("2006-01-02 15:04:05")
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", info.Ref, info.Style, url, last)
			}
			return writer.Flush()
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

type webhookShowOutput struct {
	Ref         string `json:"ref"`
	Style       string `json:"style"`
	Challenge   string `json:"challenge,omitempty"`
	URL         string `json:"url,omitempty"`
	LocalURL    string `json:"local_url,omitempty"`
	VerifyToken string `json:"verify_token,omitempty"`
	Token       string `json:"token,omitempty"`
}

func newWebhookShow(d deps) *cobra.Command {
	var showToken, asJSON bool

	cmd := &cobra.Command{
		Use:   "show <service>[/<account>]",
		Short: "Show a webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := d.core.Webhook(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			verifyToken, err := d.core.WebhookVerifyToken(cmd.Context(), info.Ref)
			if err != nil {
				return err
			}
			token := ""
			if showToken && info.Style == "token" {
				if !outputIsTerminal(d.stdout) {
					return errors.New("the token is only shown in a terminal: run this command yourself")
				}
				token, err = d.core.WebhookSigningSecret(cmd.Context(), info.Ref)
				if err != nil {
					return err
				}
			}

			if asJSON {
				return writeJSON(d.stdout, webhookShowOutput{
					Ref:         info.Ref,
					Style:       info.Style,
					Challenge:   info.Challenge,
					URL:         info.URL,
					LocalURL:    info.LocalURL,
					VerifyToken: verifyToken,
					Token:       token,
				})
			}
			fmt.Fprintf(d.stdout, "Ref: %s\n", info.Ref)
			fmt.Fprintf(d.stdout, "Style: %s\n", info.Style)
			if info.URL != "" {
				fmt.Fprintf(d.stdout, "URL: %s\n", info.URL)
			} else {
				fmt.Fprintln(d.stdout, "URL: none yet: run pandora tunnel use tailscale")
			}
			if info.LocalURL != "" {
				fmt.Fprintf(d.stdout, "Local URL: %s\n", info.LocalURL)
			}
			if verifyToken != "" {
				fmt.Fprintf(d.stdout, "Verify token: %s\n", verifyToken)
			}
			if token != "" {
				fmt.Fprintf(d.stdout, "Token: %s\n", token)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&showToken, "token", false, "show the token style token")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func newWebhookRemove(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "remove <service>[/<account>]",
		Short:       "Remove a webhook",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			resolved, err := d.registry.RemoveWebhook(ref)
			if err != nil {
				return err
			}
			d.reloadWebhooks(cmd.Context(), false)
			fmt.Fprintf(d.stdout, "Webhook removed from %s. Remove the callback URL from the service's settings too.\n", resolved)
			return nil
		},
	}
}

func newWebhookTest(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "test <service>[/<account>]",
		Short: "Send a signed test delivery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := d.core.Webhook(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			result, err := d.core.TestWebhook(cmd.Context(), info.Ref)
			if err != nil {
				return err
			}
			if result.Handshake {
				fmt.Fprintf(d.stdout, "✓ verification answered at %s\n", result.URL)
			}
			fmt.Fprintf(d.stdout, "✓ signed test event delivered to %s\n", result.URL)
			fmt.Fprintf(d.stdout, "See it with: pandora events %s\n", info.Ref)
			return nil
		},
	}
}

func (d deps) reloadWebhooks(ctx context.Context, start bool) {
	if os.Getenv("PANDORA_NO_DAEMON") == "1" {
		return
	}
	var client *daemon.Client
	var err error
	if start {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			d.debugf("daemon unavailable (%v)", exeErr)
			return
		}
		ensureCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		client, err = daemon.Ensure(ensureCtx, d.paths, exe, buildID())
	} else {
		client, err = daemon.Dial(ctx, d.paths.DaemonSocket())
	}
	if err != nil {
		d.debugf("daemon unavailable (%v)", err)
		return
	}
	state, err := client.ReloadWebhooks(ctx)
	if err != nil {
		d.debugf("could not reload webhooks: %v", err)
		return
	}
	if state.Error != "" {
		fmt.Fprintf(d.stderr, "warning: %s\n", state.Error)
	}
}

func findWebhookInfo(infos []core.WebhookInfo, ref string) (core.WebhookInfo, bool) {
	for _, info := range infos {
		if info.Ref == ref {
			return info, true
		}
	}
	return core.WebhookInfo{}, false
}

func outputIsTerminal(w io.Writer) bool {
	reader, ok := w.(io.Reader)
	if !ok {
		return false
	}
	return isTerminal(reader)
}
