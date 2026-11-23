package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/webui"
)

func newUI(d deps) *cobra.Command {
	var (
		port      int
		noBrowser bool
	)
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Open the local Pandora UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := webui.New(webui.Options{
				Core:     d.core,
				Registry: d.registry,
				CallLog:  guard.NewLog(d.paths.CallLog()),
				Version:  appVersion(),
			})
			url, done, err := server.Start(cmd.Context(), port)
			if err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Pandora UI: %s\n", url)
			fmt.Fprintln(d.stdout, "Press Ctrl+C to stop.")
			if !noBrowser {
				if open := defaultOpenBrowser(); open != nil {
					_ = open(url)
				}
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case err := <-done:
				return err
			}
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port to listen on (0 = random)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open a browser")
	return cmd
}
