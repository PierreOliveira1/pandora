package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/install"
)

func newInstall(d deps) *cobra.Command {
	var asJSON, mcp bool

	cmd := &cobra.Command{
		Use:   "install [client...]",
		Short: "Install the Pandora skill or MCP server in your AI clients",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			clients, err := parseClients(args)
			if err != nil {
				return err
			}
			reports, err := d.core.Install(cmd.Context(), clients, core.InstallOptions{MCP: mcp})
			if reports == nil {
				reports = []core.InstallReport{}
			}

			if asJSON {
				if writeErr := writeJSON(d.stdout, reports); writeErr != nil {
					return writeErr
				}
				return err
			}
			for _, report := range reports {
				fmt.Fprintln(d.stdout, installReportLine(report))
			}
			if err != nil {
				return err
			}
			if !commandOnPath(d.installEnv) && hasSkillInstall(reports) {
				fmt.Fprintf(d.stdout, "warning: pandora is not on your PATH; add %s to PATH so AI clients can run it\n", executableDir())
			}
			fmt.Fprintln(d.stdout, "Restart your AI clients to load Pandora.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	cmd.Flags().BoolVar(&mcp, "mcp", false, "install Pandora as an MCP server")
	return cmd
}

func parseClients(args []string) ([]install.Client, error) {
	clients := make([]install.Client, 0, len(args))
	for _, arg := range args {
		client, err := install.ParseClient(arg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func installReportLine(report core.InstallReport) string {
	if report.Mode == "mcp" {
		return fmt.Sprintf("%s: MCP server added to %s", report.Client, report.MCPConfig)
	}
	status := "unchanged"
	if report.SkillChanged {
		status = "updated"
	}
	line := fmt.Sprintf("%s: skill at %s (%s)", report.Client, report.SkillDir, status)
	if report.PermissionFile != "" {
		line += "; pandora allowed in " + report.PermissionFile
	}
	if report.PermissionNote != "" {
		line += "; " + report.PermissionNote
	}
	return line
}

func hasSkillInstall(reports []core.InstallReport) bool {
	for _, report := range reports {
		if report.Mode == "skill" {
			return true
		}
	}
	return false
}

func commandOnPath(env install.Env) bool {
	if env.LookPath == nil {
		return false
	}
	_, err := env.LookPath("pandora")
	return err == nil
}

func executableDir() string {
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return "the pandora binary directory"
	}
	return filepath.Dir(executable)
}
