package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
)

func newStatus(d deps) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status [ref]",
		Short: "Check the connection to accounts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backend := d.backend(cmd.Context())
			var infos []core.StatusInfo
			if len(args) == 1 {
				infos = []core.StatusInfo{backend.Status(cmd.Context(), args[0])}
			} else {
				infos = backend.StatusAll(cmd.Context())
			}

			if asJSON {
				if infos == nil {
					infos = []core.StatusInfo{}
				}
				return writeJSON(d.stdout, infos)
			}

			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "ACCOUNT\tSTATUS\tTOOLS\tLATENCY")
			for _, info := range infos {
				status := "ok"
				switch {
				case info.NeedsLogin:
					status = "needs login"
				case !info.OK:
					status = "error: " + shortError(info.Error)
				}
				fmt.Fprintf(writer, "%s\t%s\t%d\t%dms\n", info.Ref, status, info.Tools, info.Latency.Milliseconds())
			}
			return writer.Flush()
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func shortError(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	if line == "" {
		return "unknown error"
	}
	const maxLength = 80
	if len(line) > maxLength {
		return line[:maxLength-3] + "..."
	}
	return line
}
