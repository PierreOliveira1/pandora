package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

type clientKey struct{}

func WithClient(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, clientKey{}, name)
}

func ClientFrom(ctx context.Context) string {
	name, _ := ctx.Value(clientKey{}).(string)
	if name == "" {
		return "unknown"
	}
	return name
}

type BlockedError struct {
	Reason string
}

func (e *BlockedError) Error() string {
	return e.Reason
}

func (c *Core) checkPolicy(ctx context.Context, ref registry.Ref, account *config.Account, toolName string) error {
	var tool *mcp.Tool
	if account.ReadOnly {
		found, err := c.findTool(ctx, ref, toolName)
		if err != nil {
			return err
		}
		tool = found
	}
	decision := guard.Check(ref.String(), account, tool, toolName)
	if !decision.Allowed {
		return &BlockedError{Reason: decision.Reason}
	}
	return nil
}

func (c *Core) findTool(ctx context.Context, ref registry.Ref, toolName string) (*mcp.Tool, error) {
	if catalog, err := c.loadCatalog(ref.Service); err == nil && catalog != nil {
		for _, tool := range catalog.Tools {
			if tool.Name == toolName {
				return tool, nil
			}
		}
	}
	conn, err := c.open(ctx, ref.String())
	if err != nil {
		return nil, err
	}
	tools, err := conn.src.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing tools of %s: %w", ref, err)
	}
	for _, tool := range tools {
		if tool.Name == toolName {
			return tool, nil
		}
	}
	return nil, nil
}

func (c *Core) recordCall(ctx context.Context, ref, toolName string, started time.Time, result *mcp.CallToolResult, err error) {
	if c.callLog == nil {
		return
	}
	entry := guard.Entry{
		Time:       started,
		Ref:        ref,
		Tool:       toolName,
		Client:     ClientFrom(ctx),
		DurationMs: time.Since(started).Milliseconds(),
		OK:         err == nil && result != nil && !result.IsError,
	}
	var blocked *BlockedError
	switch {
	case errors.As(err, &blocked):
		entry.Blocked = true
		entry.Error = blocked.Reason
	case err != nil:
		entry.Error = err.Error()
	case result != nil && result.IsError:
		entry.Error = resultText(result)
	}
	_ = c.callLog.Append(entry)
}

func resultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
			return text.Text
		}
	}
	return "tool returned an error"
}
