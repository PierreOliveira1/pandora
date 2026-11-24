package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
)

func newTunnel(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Expose webhooks to the internet",
	}
	cmd.AddCommand(newTunnelUse(d), newTunnelStatus(d), newTunnelOff(d))
	return cmd
}

func newTunnelUse(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "use tailscale | use url <https-base>",
		Short:       "Choose how webhooks reach this machine",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			base := ""
			if len(args) == 2 {
				base = args[1]
			}
			switch provider {
			case "tailscale":
				if base != "" {
					return errors.New("pandora tunnel use tailscale does not take a URL")
				}
			case "url":
				if base == "" {
					return errors.New("usage: pandora tunnel use url <https-base>")
				}
			default:
				return fmt.Errorf("unknown tunnel %s: use tailscale or url", provider)
			}

			before, _ := d.core.Webhooks(cmd.Context())
			beforeURLs := make(map[string]string, len(before))
			for _, info := range before {
				beforeURLs[info.Ref] = info.URL
			}

			info, err := d.core.UseTunnel(cmd.Context(), provider, base)
			if err != nil {
				return err
			}
			d.reloadWebhooks(cmd.Context(), true)
			fmt.Fprintf(d.stdout, "Public address: %s\n", info.PublicURL)

			after, err := d.core.Webhooks(cmd.Context())
			if err != nil {
				return err
			}
			changed := make([]core.WebhookInfo, 0)
			for _, webhook := range after {
				if webhook.URL != "" && beforeURLs[webhook.Ref] != webhook.URL {
					changed = append(changed, webhook)
				}
			}
			if len(changed) > 0 {
				fmt.Fprintln(d.stdout, "Callback URLs changed:")
				for _, webhook := range changed {
					fmt.Fprintf(d.stdout, "  %s  %s\n", webhook.Ref, webhook.URL)
				}
				fmt.Fprintln(d.stdout, "Update them in each service.")
			}
			return nil
		},
	}
}

func newTunnelStatus(d deps) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the webhook tunnel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := d.core.TunnelStatus(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(d.stdout, info)
			}
			provider := info.Provider
			if provider == "" {
				provider = "none"
			}
			publicURL := info.PublicURL
			if publicURL == "" {
				publicURL = "none"
			}
			exposed := "no"
			if info.Exposed {
				exposed = "yes"
			}
			fmt.Fprintf(d.stdout, "provider: %s\n", provider)
			fmt.Fprintf(d.stdout, "public URL: %s\n", publicURL)
			fmt.Fprintf(d.stdout, "port: %d\n", info.Port)
			fmt.Fprintf(d.stdout, "exposed: %s\n", exposed)
			if info.Problem != "" {
				fmt.Fprintf(d.stdout, "problem: %s\n", info.Problem)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func newTunnelOff(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "off",
		Short:       "Stop exposing webhooks",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := d.core.StopTunnel(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(d.stdout, "Tunnel off. Webhooks stop receiving events from the internet.")
			return nil
		},
	}
}
