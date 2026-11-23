package gateway

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
)

func (g *Gateway) addDirectTools(ctx context.Context, server *mcp.Server, overview core.Overview) {
	services := make([]core.ServiceView, 0)
	for _, service := range overview.Services {
		if service.Expose == config.ExposeDirect && len(service.Accounts) > 0 {
			services = append(services, service)
		}
	}
	if len(services) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	type directResult struct {
		service core.ServiceView
		tools   []*mcp.Tool
		err     error
	}
	results := make(chan directResult, len(services))
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service core.ServiceView) {
			defer wg.Done()
			catalog, err := g.backend.Tools(ctx, service.Name, false)
			var tools []*mcp.Tool
			if err == nil && catalog != nil {
				tools = catalog.Tools
			}
			results <- directResult{service: service, tools: tools, err: err}
		}(service)
	}
	wg.Wait()
	close(results)

	collected := make([]directResult, 0, len(services))
	for result := range results {
		collected = append(collected, result)
	}
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].service.Name < collected[j].service.Name
	})

	used := map[string]bool{
		"pandora_accounts":      true,
		"pandora_search_tools":  true,
		"pandora_describe_tool": true,
		"pandora_call":          true,
	}
	for _, result := range collected {
		if result.err != nil {
			g.log.Warn("skipping direct-mode tools", "service", result.service.Name, "error", result.err)
			continue
		}
		g.addDirectServiceTools(server, result.service, result.tools, used)
	}
}

func (g *Gateway) addDirectServiceTools(server *mcp.Server, service core.ServiceView, tools []*mcp.Tool, used map[string]bool) {
	for _, tool := range sortedTools(tools) {
		originalName := tool.Name
		name := directName(service.Name, originalName, used)
		schema, accountArg := directSchema(service, tool.InputSchema)
		description := "[" + service.Name + "] " + tool.Description
		annotations := copyAnnotations(tool.Annotations)
		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: schema,
			Annotations: annotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, err := objectArguments(req)
			if err != nil {
				return toolError("invalid arguments: " + err.Error()), nil
			}
			ref := service.Name
			if raw, ok := args[accountArg]; ok {
				account, ok := raw.(string)
				if !ok {
					return toolError("account must be a string"), nil
				}
				delete(args, accountArg)
				if account != "" {
					ref = service.Name + "/" + account
				}
			}
			result, err := g.backend.Call(callContext(ctx, req), ref, originalName, args)
			if err != nil {
				return toolError(err.Error()), nil
			}
			return result, nil
		})
	}
}

func directSchema(service core.ServiceView, original any) (map[string]any, string) {
	schema := map[string]any{}
	if original != nil {
		if data, err := json.Marshal(original); err == nil {
			_ = json.Unmarshal(data, &schema)
		}
	}
	if schema == nil {
		schema = map[string]any{}
	}
	schema["type"] = "object"
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	schema["properties"] = properties
	accountArg := "account"
	if _, exists := properties[accountArg]; exists {
		accountArg = "_account"
	}
	accounts := make([]string, len(service.Accounts))
	for i, account := range service.Accounts {
		accounts[i] = account.Name
	}
	description := "Account to use."
	if service.DefaultAccount != "" {
		description += " Default: " + service.DefaultAccount
	}
	properties[accountArg] = map[string]any{
		"type":        "string",
		"enum":        accounts,
		"description": description,
	}
	if service.DefaultAccount == "" && len(service.Accounts) > 1 {
		required := requiredValues(schema["required"])
		required = append(required, accountArg)
		schema["required"] = required
	}
	return schema, accountArg
}

func requiredValues(value any) []any {
	switch typed := value.(type) {
	case []any:
		return append([]any(nil), typed...)
	case []string:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	default:
		return nil
	}
}

func objectArguments(req *mcp.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return args, nil
	}
	if string(req.Params.Arguments) == "null" {
		return args, nil
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	return args, nil
}

func copyAnnotations(annotations *mcp.ToolAnnotations) *mcp.ToolAnnotations {
	if annotations == nil {
		return nil
	}
	copied := *annotations
	return &copied
}
