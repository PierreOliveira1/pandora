package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/guard"
)

func newLog(d deps) *cobra.Command {
	var (
		limit    int
		account  string
		onlyErrs bool
		asJSON   bool
	)
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show the latest tool calls",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return errors.New("-n must be zero or positive")
			}
			entries, err := guard.NewLog(d.paths.CallLog()).Tail(limit, guard.Filter{
				Ref:        account,
				OnlyErrors: onlyErrs,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(d.stdout, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(d.stdout, "No calls yet.")
				return nil
			}
			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "TIME\tCLIENT\tACCOUNT\tTOOL\tRESULT\tDURATION")
			for _, entry := range entries {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%dms\n",
					logTime(entry.Time), entry.Client, entry.Ref, entry.Tool, logResult(entry), entry.DurationMs)
			}
			return writer.Flush()
		},
	}
	cmd.Flags().IntVarP(&limit, "n", "n", 50, "number of calls to show")
	cmd.Flags().StringVar(&account, "account", "", "filter by account or service")
	cmd.Flags().BoolVar(&onlyErrs, "errors", false, "show only failed or blocked calls")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func logTime(value time.Time) string {
	local := value.Local()
	now := time.Now().Local()
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("15:04:05")
	}
	return local.Format("2006-01-02 15:04:05")
}

func logResult(entry guard.Entry) string {
	if entry.Blocked {
		return "blocked"
	}
	if !entry.OK {
		if entry.Error == "" {
			return "error"
		}
		return "error: " + firstLine(entry.Error)
	}
	return "ok"
}
