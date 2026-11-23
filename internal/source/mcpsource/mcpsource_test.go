package mcpsource_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-mcpsource-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating test directory: %v\n", err)
		os.Exit(1)
	}
	fakeMCPPath = filepath.Join(dir, fakemcp.ExecutableName())

	root, err := filepath.Abs("../../..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "finding module root: %v\n", err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", fakeMCPPath, "./internal/testutil/fakemcp/cmd/fakemcp")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakemcp: %v\n%s", err, output)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestAStdioAccountListsAndCallsTheUpstreamTools(t *testing.T) {
	ctx := context.Background()
	src, err := mcpsource.OpenStdio(ctx, stdioSpec(t, "ana"))
	if err != nil {
		t.Fatalf("opening stdio account: %v", err)
	}
	defer src.Close()

	tools, err := src.ListTools(ctx)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}
	if len(tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(tools))
	}
	if got := src.Instructions(); got != "Fake server for tests" {
		t.Fatalf("instructions = %q, want %q", got, "Fake server for tests")
	}

	result, err := src.CallTool(ctx, "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("calling echo: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if text.Text != "hello" {
		t.Fatalf("echo returned %q, want %q", text.Text, "hello")
	}
}

func TestEachAccountReachesTheUpstreamWithItsOwnCredentials(t *testing.T) {
	ctx := context.Background()
	first, err := mcpsource.OpenStdio(ctx, stdioSpec(t, "ana"))
	if err != nil {
		t.Fatalf("opening first account: %v", err)
	}
	defer first.Close()
	second, err := mcpsource.OpenStdio(ctx, stdioSpec(t, "bob"))
	if err != nil {
		t.Fatalf("opening second account: %v", err)
	}
	defer second.Close()

	assertIdentity(t, ctx, first, "ana")
	assertIdentity(t, ctx, second, "bob")
}

func TestAnHTTPAccountSendsItsSecretHeaders(t *testing.T) {
	server := httptest.NewServer(fakemcp.NewHTTPHandler("tok"))
	defer server.Close()

	spec := httpSpec(server.URL, map[string]string{
		"Authorization": "Bearer tok",
		"X-Account":     "ana",
	})
	src, err := mcpsource.OpenHTTP(context.Background(), spec)
	if err != nil {
		t.Fatalf("opening HTTP account: %v", err)
	}
	defer src.Close()

	assertIdentity(t, context.Background(), src, "ana")
}

func TestAnHTTPServerAnswering401ReportsThatLoginIsNeeded(t *testing.T) {
	server := httptest.NewServer(fakemcp.NewHTTPHandler("tok"))
	defer server.Close()

	spec := httpSpec(server.URL, map[string]string{
		"Authorization": "Bearer wrong",
		"X-Account":     "ana",
	})
	_, err := mcpsource.OpenHTTP(context.Background(), spec)
	if err == nil {
		t.Fatal("opening HTTP account succeeded, want a login error")
	}
	var authErr *source.AuthRequiredError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %v, want *source.AuthRequiredError", err)
	}
	if authErr.Ref != "fake/a" {
		t.Fatalf("ref = %q, want %q", authErr.Ref, "fake/a")
	}
}

func stdioSpec(t *testing.T, identity string) source.Spec {
	t.Helper()
	return source.Spec{
		Service:    "fake",
		Account:    "a",
		Kind:       config.KindMCPStdio,
		Svc:        &config.Service{Kind: config.KindMCPStdio, Command: []string{fakeMCPPath}},
		Acc:        &config.Account{},
		Env:        map[string]string{"FAKE_ACCOUNT": identity},
		HTTPClient: &http.Client{},
		LogPath:    filepath.Join(t.TempDir(), "upstream.log"),
	}
}

func httpSpec(url string, headers map[string]string) source.Spec {
	return source.Spec{
		Service: "fake",
		Account: "a",
		Kind:    config.KindMCPHTTP,
		Svc: &config.Service{
			Kind: config.KindMCPHTTP,
			Auth: config.AuthHeader,
			URL:  url,
		},
		Acc:        &config.Account{},
		URL:        url,
		Headers:    headers,
		HTTPClient: &http.Client{},
	}
}

func assertIdentity(t *testing.T, ctx context.Context, src source.Source, want string) {
	t.Helper()
	result, err := src.CallTool(ctx, "whoami", nil)
	if err != nil {
		t.Fatalf("calling whoami: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if text.Text != want {
		t.Fatalf("whoami = %q, want %q", text.Text, want)
	}
}
