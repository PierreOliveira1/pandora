package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/core"
)

func (g *Gateway) addCompactTools(server *mcp.Server) {
	server.AddTool(&mcp.Tool{
		Name:        "pandora_accounts",
		Description: "List every connected service and account, with their descriptions and which accounts are read-only. Use it to pick the account to work with.",
		InputSchema: map[string]any{"type": "object"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		overview, err := g.core.Overview(callContext(ctx, req))
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolText(accountsText(overview)), nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "pandora_search_tools",
		Description: "Find tools by words in their name or description across all services, or list the tools of one service. Use it before pandora_call when you do not know the exact tool name.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]any{"type": "string"},
				"service": map[string]any{"type": "string"},
				"limit":   map[string]any{"type": "integer"},
			},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Query   string `json:"query"`
			Service string `json:"service"`
			Limit   int    `json:"limit"`
		}
		if err := decodeArguments(req, &args); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		hits, total, err := g.backend.Search(callContext(ctx, req), args.Query, args.Service, limit)
		if err != nil {
			return toolError(err.Error()), nil
		}
		if len(hits) == 0 {
			return toolText("No tools found."), nil
		}
		lines := make([]string, len(hits))
		for i, hit := range hits {
			lines[i] = hit.Service + "  " + hit.Tool + "  " + hit.Summary
		}
		if more := total - len(hits); more > 0 {
			lines = append(lines, fmt.Sprintf("%d more results, refine the query", more))
		}
		return toolText(strings.Join(lines, "\n")), nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "pandora_describe_tool",
		Description: "Show the full description and input arguments of a tool. Use it before pandora_call to build the arguments.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service": map[string]any{"type": "string"},
				"tool":    map[string]any{"type": "string"},
			},
			"required": []string{"service", "tool"},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Service string `json:"service"`
			Tool    string `json:"tool"`
		}
		if err := decodeArguments(req, &args); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
		if args.Service == "" || args.Tool == "" {
			return toolError("service and tool are required"), nil
		}
		tool, err := g.backend.Describe(callContext(ctx, req), args.Service, args.Tool)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolText(describeText(tool)), nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "pandora_call",
		Description: "Run a tool on an account. account is \"service/account\", or just \"service\" for its default account; arguments follow the tool input schema.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"account":   map[string]any{"type": "string"},
				"tool":      map[string]any{"type": "string"},
				"arguments": map[string]any{"type": "object"},
			},
			"required": []string{"account", "tool"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Account   string         `json:"account"`
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := decodeArguments(req, &args); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
		if args.Account == "" || args.Tool == "" {
			return toolError("account and tool are required"), nil
		}
		result, err := g.backend.Call(callContext(ctx, req), args.Account, args.Tool, args.Arguments)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return result, nil
	})
}

func decodeArguments(req *mcp.CallToolRequest, target any) error {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	if string(req.Params.Arguments) == "null" {
		return nil
	}
	return json.Unmarshal(req.Params.Arguments, target)
}

func accountsText(overview core.Overview) string {
	if len(overview.Services) == 0 {
		return "No services are connected."
	}
	lines := make([]string, 0)
	for _, service := range overview.Services {
		header := fmt.Sprintf("%s (%s, %s)", service.Name, service.Kind, service.Expose)
		lines = append(lines, header)
		if service.Description != "" {
			lines = append(lines, "  "+service.Description)
		}
		if len(service.Accounts) == 0 {
			lines = append(lines, "  accounts: none")
			continue
		}
		accounts := make([]string, len(service.Accounts))
		for i, account := range service.Accounts {
			label := accountLabel(account)
			if account.Description != "" {
				label += " — " + account.Description
			}
			accounts[i] = label
		}
		lines = append(lines, "  accounts: "+strings.Join(accounts, "; "))
	}
	return strings.Join(lines, "\n")
}

func describeText(tool *mcp.Tool) string {
	readOnly := tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	destructive := tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
	schema, err := json.MarshalIndent(tool.InputSchema, "", "  ")
	schemaText := string(schema)
	if err != nil {
		schemaText = fmt.Sprint(tool.InputSchema)
	}
	var b strings.Builder
	b.WriteString(tool.Name)
	if tool.Description != "" {
		b.WriteString("\n" + tool.Description)
	}
	b.WriteString("\nread-only: " + yesNo(readOnly))
	b.WriteString("\ndestructive: " + yesNo(destructive))
	b.WriteString("\ninput schema:\n" + schemaText)
	return b.String()
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
