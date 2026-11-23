package guard

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func TestADenyRuleBlocksAToolEvenWhenItIsAllowed(t *testing.T) {
	account := &config.Account{Allow: []string{"*"}, Deny: []string{"delete-*"}}
	decision := Check("todoist/work", account, &mcp.Tool{Name: "delete-user"}, "delete-user")
	if decision.Allowed {
		t.Fatal("Check() allowed a denied tool")
	}
	want := `tool delete-user is blocked for todoist/work by the deny rule "delete-*" (change it with pandora policy todoist/work)`
	if decision.Reason != want {
		t.Errorf("Reason = %q, want %q", decision.Reason, want)
	}
}

func TestAnAllowListBlocksEveryToolItDoesNotName(t *testing.T) {
	account := &config.Account{Allow: []string{"find-*", "get-*"}}
	allowed := Check("todoist/work", account, &mcp.Tool{Name: "get-user"}, "get-user")
	if !allowed.Allowed {
		t.Fatalf("Check() blocked get-user: %s", allowed.Reason)
	}
	blocked := Check("todoist/work", account, &mcp.Tool{Name: "create-user"}, "create-user")
	if blocked.Allowed {
		t.Fatal("Check() allowed create-user")
	}
	want := "tool create-user is not in the allow list of todoist/work (change it with pandora policy todoist/work)"
	if blocked.Reason != want {
		t.Errorf("Reason = %q, want %q", blocked.Reason, want)
	}
}

func TestAReadOnlyAccountOnlyRunsToolsMarkedReadOnly(t *testing.T) {
	account := &config.Account{ReadOnly: true}
	readTool := &mcp.Tool{Name: "get-user", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
	if decision := Check("todoist/work", account, readTool, "get-user"); !decision.Allowed {
		t.Fatalf("Check() blocked a read-only tool: %s", decision.Reason)
	}
	writeTool := &mcp.Tool{Name: "create-user"}
	decision := Check("todoist/work", account, writeTool, "create-user")
	if decision.Allowed {
		t.Fatal("Check() allowed a tool without read-only annotations")
	}
	want := "todoist/work is read-only and tool create-user may change data (change it with pandora policy todoist/work)"
	if decision.Reason != want {
		t.Errorf("Reason = %q, want %q", decision.Reason, want)
	}
	if decision := Check("todoist/work", account, nil, "unknown"); decision.Allowed {
		t.Fatal("Check() allowed a nil tool on a read-only account")
	}
}

func TestTheCallLogKeepsTheLatestCallsAcrossRotation(t *testing.T) {
	log := NewLogWithLimit(filepath.Join(t.TempDir(), "calls.jsonl"), 1)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		err := log.Append(Entry{
			Time:       start.Add(time.Duration(i) * time.Second),
			Ref:        "todoist/work",
			Tool:       fmt.Sprintf("tool-%d", i),
			Client:     "terminal",
			DurationMs: int64(i),
			OK:         true,
		})
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}
	entries, err := log.Tail(10, Filter{})
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Tool != "tool-2" || entries[1].Tool != "tool-3" {
		t.Errorf("Tail() tools = %v, want [tool-2 tool-3]", entryTools(entries))
	}
}

func TestTheClientIsDetectedFromTheAIsEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "claude", env: map[string]string{"CLAUDECODE": "1"}, want: "claude-code"},
		{name: "opencode", env: map[string]string{"OPENCODE": "1"}, want: "opencode"},
		{name: "codex sandbox", env: map[string]string{"CODEX_SANDBOX": "1"}, want: "codex"},
		{name: "codex home", env: map[string]string{"CODEX_HOME": "/tmp/codex"}, want: "codex"},
		{name: "cursor trace", env: map[string]string{"CURSOR_TRACE_ID": "1"}, want: "cursor"},
		{name: "cursor agent", env: map[string]string{"CURSOR_AGENT": "1"}, want: "cursor"},
		{name: "terminal", env: map[string]string{}, want: "terminal"},
		{name: "override", env: map[string]string{"CLAUDECODE": "1", "PANDORA_CLIENT": "custom"}, want: "custom"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := DetectClient(func(name string) string { return test.env[name] })
			if got != test.want {
				t.Errorf("DetectClient() = %q, want %q", got, test.want)
			}
		})
	}
}

func entryTools(entries []Entry) []string {
	tools := make([]string, len(entries))
	for i, entry := range entries {
		tools[i] = entry.Tool
	}
	return tools
}
