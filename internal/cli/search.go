package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
)

type searchJSON struct {
	Results []core.SearchHit `json:"results"`
	Total   int              `json:"total"`
}

func newSearch(d deps) *cobra.Command {
	var service string
	var limit int
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "search <query...>",
		Short: "Search tools by name and description",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backend := d.backend(cmd.Context())
			query := strings.Join(args, " ")
			hits, total, err := backend.Search(cmd.Context(), query, service, limit)
			if err != nil {
				return err
			}

			if asJSON {
				return writeJSON(d.stdout, searchJSON{Results: hits, Total: total})
			}
			if len(hits) == 0 {
				fmt.Fprintf(d.stdout, "No tools match %q.\n", query)
				return nil
			}

			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			for _, hit := range hits {
				fmt.Fprintf(writer, "%s\t%s\t%s\n", hit.Service, hit.Tool, hit.Summary)
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			if remaining := total - len(hits); remaining > 0 {
				fmt.Fprintf(d.stdout, "%d more results: refine the query or use --limit\n", remaining)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&service, "service", "", "search only this service")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of results")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}
