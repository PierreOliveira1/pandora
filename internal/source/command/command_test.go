package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/source"
)

func TestACommandToolRunsWithItsArgumentsAndReturnsTheOutput(t *testing.T) {
	src := openTestSource(t, []config.CommandTool{{
		Name:    "greet",
		Command: []string{"/bin/echo", "hello", "{{name}}"},
		Params: map[string]config.CommandParam{
			"name": {Type: "string", Required: true},
		},
	}})

	result, err := src.CallTool(context.Background(), "greet", map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := strings.TrimSpace(resultText(result)); got != "hello world" {
		t.Errorf("output = %q, want hello world", got)
	}
}

func TestOptionalArgumentsThatAreMissingDropTheirPartOfTheCommand(t *testing.T) {
	src := openTestSource(t, []config.CommandTool{{
		Name:    "list",
		Command: []string{"/bin/echo", "start", "--limit={{limit}}", "end"},
		Params: map[string]config.CommandParam{
			"limit": {Type: "integer"},
		},
	}})

	without, err := src.CallTool(context.Background(), "list", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() without limit error = %v", err)
	}
	if got := strings.TrimSpace(resultText(without)); got != "start end" {
		t.Errorf("output without limit = %q, want start end", got)
	}
	with, err := src.CallTool(context.Background(), "list", map[string]any{"limit": 5})
	if err != nil {
		t.Fatalf("CallTool() with limit error = %v", err)
	}
	if got := strings.TrimSpace(resultText(with)); got != "start --limit=5 end" {
		t.Errorf("output with limit = %q, want start --limit=5 end", got)
	}
}

func TestArgumentsAreNeverInterpretedByAShell(t *testing.T) {
	target := filepath.Join(t.TempDir(), "owned")
	payload := fmt.Sprintf("$(touch %s); echo injected", target)
	src := openTestSource(t, []config.CommandTool{{
		Name:    "echo-value",
		Command: []string{"/bin/echo", "{{value}}"},
		Params: map[string]config.CommandParam{
			"value": {Type: "string", Required: true},
		},
	}})

	result, err := src.CallTool(context.Background(), "echo-value", map[string]any{"value": payload})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() result is an error: %s", resultText(result))
	}
	if got := strings.TrimSpace(resultText(result)); got != payload {
		t.Errorf("output = %q, want the literal value %q", got, payload)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(%q) error = %v, want no file created", target, err)
	}
}

func TestAFailingCommandComesBackAsAToolErrorWithItsStderr(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	src := openTestSource(t, []config.CommandTool{{
		Name:    "list-missing",
		Command: []string{"/bin/ls", missing},
	}})

	result, err := src.CallTool(context.Background(), "list-missing", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() IsError = false, want true")
	}
	text := resultText(result)
	if !strings.HasPrefix(text, "exit status") {
		t.Errorf("result text = %q, want an exit status", text)
	}
	if !strings.Contains(text, missing) {
		t.Errorf("result text = %q, want the stderr with %s", text, missing)
	}
}

func TestACommandThatTakesTooLongIsStopped(t *testing.T) {
	src := openTestSource(t, []config.CommandTool{{
		Name:       "slow",
		Command:    []string{"/bin/sleep", "5"},
		TimeoutSec: 1,
	}})

	result, err := src.CallTool(context.Background(), "slow", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("CallTool() IsError = false, want true")
	}
	if got := resultText(result); got != "timed out after 1s" {
		t.Errorf("result text = %q, want timed out after 1s", got)
	}
}

func TestAValueThatLooksLikeAFlagIsRefusedUnlessItComesAfterDoubleDash(t *testing.T) {
	src := openTestSource(t, []config.CommandTool{
		{
			Name:    "echo",
			Command: []string{"/bin/echo", "{{value}}"},
			Params: map[string]config.CommandParam{
				"value": {Type: "string", Required: true},
			},
		},
		{
			Name:    "echo-after",
			Command: []string{"/bin/echo", "--", "{{value}}"},
			Params: map[string]config.CommandParam{
				"value": {Type: "string", Required: true},
			},
		},
		{
			Name:    "count",
			Command: []string{"/bin/echo", "{{n}}"},
			Params: map[string]config.CommandParam{
				"n": {Type: "integer", Required: true},
			},
		},
		{
			Name:    "tags",
			Command: []string{"/bin/echo", "{{tags}}"},
			Params: map[string]config.CommandParam{
				"tags": {Type: "array", Required: true},
			},
		},
		{
			Name:    "mixed",
			Command: []string{"/bin/echo", "--limit={{value}}"},
			Params: map[string]config.CommandParam{
				"value": {Type: "string", Required: true},
			},
		},
	})

	refused, err := src.CallTool(context.Background(), "echo", map[string]any{"value": "-rf"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !refused.IsError {
		t.Fatal("CallTool() allowed a flag-like value")
	}
	if got := resultText(refused); got != `argument value cannot start with "-"` {
		t.Errorf("result text = %q, want the flag error", got)
	}

	after, err := src.CallTool(context.Background(), "echo-after", map[string]any{"value": "-rf"})
	if err != nil {
		t.Fatalf("CallTool() after -- error = %v", err)
	}
	if after.IsError || !strings.Contains(resultText(after), "-rf") {
		t.Errorf("CallTool() after -- = %q, want the value", resultText(after))
	}

	negative, err := src.CallTool(context.Background(), "count", map[string]any{"n": -5})
	if err != nil {
		t.Fatalf("CallTool() with a negative number error = %v", err)
	}
	if negative.IsError || strings.TrimSpace(resultText(negative)) != "-5" {
		t.Errorf("CallTool() with a negative number = %q, want -5", resultText(negative))
	}

	array, err := src.CallTool(context.Background(), "tags", map[string]any{"tags": []any{"-a"}})
	if err != nil {
		t.Fatalf("CallTool() with a flag-like array item error = %v", err)
	}
	if !array.IsError || resultText(array) != `argument tags cannot start with "-"` {
		t.Errorf("CallTool() with a flag-like array item = %q, want the flag error", resultText(array))
	}

	mixed, err := src.CallTool(context.Background(), "mixed", map[string]any{"value": "-rf"})
	if err != nil {
		t.Fatalf("CallTool() with a mixed placeholder error = %v", err)
	}
	if mixed.IsError || strings.TrimSpace(resultText(mixed)) != "--limit=-rf" {
		t.Errorf("CallTool() with a mixed placeholder = %q, want --limit=-rf", resultText(mixed))
	}
}

func openTestSource(t *testing.T, tools []config.CommandTool) *Source {
	t.Helper()
	value, err := Open(context.Background(), source.Spec{
		Svc: &config.Service{Kind: config.KindCommand, Tools: tools},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return value.(*Source)
}

func resultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}
