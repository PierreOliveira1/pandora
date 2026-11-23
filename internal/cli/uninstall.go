package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
)

func newUninstall(d deps) *cobra.Command {
	var asJSON, mcp bool

	cmd := &cobra.Command{
		Use:   "uninstall [client...]",
		Short: "Remove Pandora from your AI clients",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := parseClients(args)
			if err != nil {
				return err
			}
			reports, err := d.core.Uninstall(cmd.Context(), clients)
			if reports == nil {
				reports = []core.InstallReport{}
			}

			if asJSON {
				if writeErr := writeJSON(d.stdout, reports); writeErr != nil {
					return writeErr
				}
				return err
			}
			if len(reports) == 0 && err == nil {
				fmt.Fprintln(d.stdout, "No AI clients have Pandora installed.")
				return nil
			}
			for _, report := range reports {
				fmt.Fprintln(d.stdout, uninstallReportLine(report, mcp))
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().BoolVar(&mcp, "mcp", false, "show the MCP server removal")
	return cmd
}

func uninstallReportLine(report core.InstallReport, mcp bool) string {
	if mcp {
		return fmt.Sprintf("%s: MCP server removed from %s", report.Client, report.MCPConfig)
	}
	parts := make([]string, 0, 2)
	status := "unchanged"
	if report.SkillChanged {
		status = "removed"
	}
	parts = append(parts, fmt.Sprintf("skill at %s (%s)", report.SkillDir, status))
	if report.MCPConfig != "" {
		parts = append(parts, "MCP server at "+report.MCPConfig)
	}
	line := fmt.Sprintf("%s: %s", report.Client, strings.Join(parts, "; "))
	if report.PermissionFile != "" && report.PermissionAdded {
		line += "; pandora permission removed from " + report.PermissionFile
	}
	return line
}
