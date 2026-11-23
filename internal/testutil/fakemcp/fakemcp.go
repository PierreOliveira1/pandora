package fakemcp

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ExecutableName() string {
	if runtime.GOOS == "windows" {
		return "fakemcp.exe"
	}
	return "fakemcp"
}

func NewServer(identity string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "fakemcp",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Instructions: "Fake server for tests",
	})

	server.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echo the provided text",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: input.Text}},
		}, nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "add",
		Description: "Add two numbers and return the sum",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: strconv.FormatFloat(input.A+input.B, 'f', -1, 64)}},
		}, nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "whoami",
		Description: "Return the account identity",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: identity}},
		}, nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "fail",
		Description: "Always fail with an error",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "boom"}},
		}, nil
	})

	return server
}

func NewHTTPHandler(requiredToken string) http.Handler {
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return NewServer(r.Header.Get("X-Account"))
	}, nil)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiredToken != "" && r.Header.Get("Authorization") != "Bearer "+requiredToken {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r)
	})
}
