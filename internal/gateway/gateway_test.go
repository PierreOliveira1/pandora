package gateway_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/gateway"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-gateway-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating test directory: %v\n", err)
		os.Exit(1)
	}
	fakeMCPPath = filepath.Join(dir, fakemcp.ExecutableName())

	root, err := filepath.Abs("../..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "finding module root: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", fakeMCPPath, "./internal/testutil/fakemcp/cmd/fakemcp")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakemcp: %v\n%s\n", err, output)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type fixture struct {
	store *config.Store
	reg   *registry.Registry
	core  *core.Core
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	store := config.NewStore(config.Paths{
		ConfigDir: filepath.Join(root, "config"),
		CacheDir:  filepath.Join(root, "cache"),
		StateDir:  filepath.Join(root, "state"),
	})
	sec := secrets.NewMemoryStore()
	openers := source.Openers{}
	mcpsource.Register(openers)
	installEnv := install.Env{Home: home, GOOS: "darwin", Executable: "/opt/pandora"}
	app := core.New(core.Options{
		Store:      store,
		Secrets:    sec,
		Openers:    openers,
		Version:    "test",
		InstallEnv: installEnv,
	})
	t.Cleanup(func() {
		_ = app.Close()
	})
	return &fixture{
		store: store,
		reg:   registry.New(store, sec),
		core:  app,
	}
}

func (f *fixture) addFake(t *testing.T, service, account, identity string) {
	t.Helper()
	cfg, err := f.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	input := registry.AddInput{
		Service:   service,
		Account:   account,
		SecretEnv: map[string]string{"FAKE_ACCOUNT": identity},
	}
	if _, ok := cfg.Services[service]; !ok {
		input.Kind = config.KindMCPStdio
		input.Command = []string{fakeMCPPath}
	}
	if err := f.reg.Add(input); err != nil {
		t.Fatalf("adding %s/%s: %v", service, account, err)
	}
}

func TestInCompactModeTheAISeesOnlyTheFourPandoraToolsHoweverManyAccountsExist(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "a")
	f.addFake(t, "fake", "b", "b")
	f.addFake(t, "fake", "c", "c")
	session := connect(t, f)

	tools := listTools(t, session)
	if len(tools) != 4 {
		t.Fatalf("tools = %v, want exactly 4", toolNames(tools))
	}
	names := toolNames(tools)
	for _, want := range []string{"pandora_accounts", "pandora_search_tools", "pandora_describe_tool", "pandora_call"} {
		if !slices.Contains(names, want) {
			t.Errorf("tools = %v, want %s", names, want)
		}
	}
}

func TestTheAICanFindDescribeAndCallAToolOfAChosenAccount(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "a")
	f.addFake(t, "fake", "b", "b")
	session := connect(t, f)
	ctx := context.Background()

	search := callTool(t, ctx, session, "pandora_search_tools", map[string]any{"query": "who"})
	if search.IsError {
		t.Fatalf("search failed: %s", textOf(search))
	}
	if text := textOf(search); !strings.Contains(text, "whoami") || !strings.Contains(text, "fake") {
		t.Errorf("search text = %q, want the whoami tool", text)
	}

	describe := callTool(t, ctx, session, "pandora_describe_tool", map[string]any{
		"service": "fake",
		"tool":    "whoami",
	})
	if describe.IsError {
		t.Fatalf("describe failed: %s", textOf(describe))
	}
	if text := textOf(describe); !strings.Contains(text, "whoami") || !strings.Contains(text, "input schema") {
		t.Errorf("describe text = %q, want the tool name and schema", text)
	}

	call := callTool(t, ctx, session, "pandora_call", map[string]any{
		"account":   "fake/b",
		"tool":      "whoami",
		"arguments": map[string]any{},
	})
	if call.IsError {
		t.Fatalf("call failed: %s", textOf(call))
	}
	if got := textOf(call); got != "b" {
		t.Errorf("call text = %q, want b", got)
	}
}

func TestADirectModeServiceShowsEachToolOnceWithAnAccountArgument(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "a")
	f.addFake(t, "fake", "b", "b")
	if err := f.reg.SetExpose("fake", config.ExposeDirect); err != nil {
		t.Fatalf("setting direct mode: %v", err)
	}
	session := connect(t, f)
	ctx := context.Background()

	tools := listTools(t, session)
	var whoami *mcp.Tool
	for _, tool := range tools {
		if tool.Name == "fake__whoami" {
			whoami = tool
			break
		}
	}
	if whoami == nil {
		t.Fatalf("tools = %v, want fake__whoami", toolNames(tools))
	}
	schema := inputSchema(t, whoami)
	properties, _ := schema["properties"].(map[string]any)
	account, _ := properties["account"].(map[string]any)
	if account == nil {
		t.Fatalf("schema = %v, want an account argument", schema)
	}
	if enum := stringList(account["enum"]); !slices.Equal(enum, []string{"a", "b"}) {
		t.Errorf("account enum = %v, want [a b]", enum)
	}
	if description, _ := account["description"].(string); !strings.Contains(description, "Default: a") {
		t.Errorf("account description = %q, want the default account", description)
	}

	withAccount := callTool(t, ctx, session, "fake__whoami", map[string]any{"account": "b"})
	if withAccount.IsError || textOf(withAccount) != "b" {
		t.Errorf("call with account = %q, want b", textOf(withAccount))
	}
	withoutAccount := callTool(t, ctx, session, "fake__whoami", map[string]any{})
	if withoutAccount.IsError || textOf(withoutAccount) != "a" {
		t.Errorf("call without account = %q, want the default a", textOf(withoutAccount))
	}
}

func TestABlockedOrUnknownCallComesBackAsAToolError(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "a")
	session := connect(t, f)
	ctx := context.Background()

	unknown := callTool(t, ctx, session, "pandora_call", map[string]any{
		"account": "fake/missing",
		"tool":    "whoami",
	})
	if !unknown.IsError {
		t.Fatal("unknown account did not produce a tool error")
	}
	if text := textOf(unknown); !strings.Contains(text, "missing") {
		t.Errorf("unknown account text = %q, want the account name", text)
	}

	readOnly := true
	if _, err := f.reg.SetPolicy(registry.Ref{Service: "fake", Account: "a"}, registry.PolicyInput{ReadOnly: &readOnly}); err != nil {
		t.Fatalf("setting read-only: %v", err)
	}
	blocked := callTool(t, ctx, session, "pandora_call", map[string]any{
		"account":   "fake/a",
		"tool":      "add",
		"arguments": map[string]any{"a": 1, "b": 2},
	})
	if !blocked.IsError {
		t.Fatal("read-only account did not produce a tool error")
	}
	if text := textOf(blocked); !strings.Contains(text, "read-only") {
		t.Errorf("read-only text = %q, want a read-only error", text)
	}
}

func connect(t *testing.T, f *fixture) *mcp.ClientSession {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw := gateway.New(f.core, f.core, "test", logger)
	ctx := context.Background()
	server, err := gw.Server(ctx)
	if err != nil {
		t.Fatalf("building gateway server: %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("connecting client: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = serverSession.Close()
	})
	return session
}

func listTools(t *testing.T, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	return result.Tools
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", name, err)
	}
	return result
}

func textOf(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func inputSchema(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("schema %v is not an object", tool.InputSchema)
	}
	return schema
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
