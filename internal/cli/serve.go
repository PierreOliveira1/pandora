package cli

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/gateway"
)

type sharedBackend struct {
	d deps
}

func (b sharedBackend) Tools(ctx context.Context, service string, refresh bool) (*core.Catalog, error) {
	return b.d.backend(ctx).Tools(ctx, service, refresh)
}

func (b sharedBackend) Describe(ctx context.Context, service, tool string) (*mcp.Tool, error) {
	return b.d.backend(ctx).Describe(ctx, service, tool)
}

func (b sharedBackend) Search(ctx context.Context, query, service string, limit int) ([]core.SearchHit, int, error) {
	return b.d.backend(ctx).Search(ctx, query, service, limit)
}

func (b sharedBackend) Call(ctx context.Context, ref, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	return b.d.backend(ctx).Call(ctx, ref, tool, args)
}

func newServe(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run Pandora as an MCP server on stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			level := slog.LevelInfo
			if d.verbose != nil && *d.verbose {
				level = slog.LevelDebug
			}
			logger := slog.New(slog.NewTextHandler(d.stderr, &slog.HandlerOptions{Level: level}))
			gw := gateway.New(d.core, sharedBackend{d: d}, appVersion(), logger)
			server, err := gw.Server(cmd.Context())
			if err != nil {
				return err
			}
			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
