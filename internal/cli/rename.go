package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

func newRename(d deps) *cobra.Command {
	return &cobra.Command{
		Use:         "rename <service/account> <new-account>",
		Short:       "Rename an account or a service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			before, _ := d.core.Webhooks(cmd.Context())
			beforeURLs := make(map[string]string, len(before))
			for _, info := range before {
				beforeURLs[info.Ref] = info.URL
			}

			if ref.Account != "" {
				if err := d.registry.RenameAccount(ref, args[1]); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Renamed %s to %s/%s\n", ref.String(), ref.Service, args[1])
				warnChangedWebhook(cmd, d, beforeURLs, ref.String(), ref.Service+"/"+args[1])
				return nil
			}

			if err := d.registry.RenameService(ref.Service, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Renamed service %s to %s\n", ref.Service, args[1])
			after, err := d.core.Webhooks(cmd.Context())
			if err != nil {
				d.debugf("could not compare webhook URLs after the rename: %v", err)
				return nil
			}
			for _, info := range after {
				account, ok := strings.CutPrefix(info.Ref, args[1]+"/")
				if !ok {
					continue
				}
				oldURL, had := beforeURLs[ref.Service+"/"+account]
				if had && info.URL != "" && info.URL != oldURL {
					printWebhookWarning(d, info.Ref, info.URL)
				}
			}
			return nil
		},
	}
}

func warnChangedWebhook(cmd *cobra.Command, d deps, before map[string]string, oldRef, newRef string) {
	oldURL, had := before[oldRef]
	if !had {
		return
	}
	after, err := d.core.Webhooks(cmd.Context())
	if err != nil {
		d.debugf("could not compare webhook URLs after the rename: %v", err)
		return
	}
	info, ok := findWebhookInfo(after, newRef)
	if ok && info.URL != "" && info.URL != oldURL {
		printWebhookWarning(d, info.Ref, info.URL)
	}
}

func printWebhookWarning(d deps, ref, url string) {
	fmt.Fprintf(d.stderr, "warning: the webhook URL of %s changed to %s: update it in the service\n", ref, url)
}
