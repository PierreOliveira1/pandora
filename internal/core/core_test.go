package core_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-core-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating test directory: %v\n", err)
		os.Exit(1)
	}
	fakeMCPPath = filepath.Join(dir, fakemcp.ExecutableName())

	root, err := filepath.Abs("../..")
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

type fixture struct {
	store      *config.Store
	secrets    secrets.Store
	openers    source.Openers
	installEnv install.Env
	home       string
	core       *core.Core
	reg        *registry.Registry
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
	installEnv := install.Env{Home: home, GOOS: "darwin"}
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
		store:      store,
		secrets:    sec,
		openers:    openers,
		installEnv: installEnv,
		home:       home,
		core:       app,
		reg:        registry.New(store, sec),
	}
}

func (f *fixture) addFake(t *testing.T, service, account, identity string) {
	t.Helper()
	err := f.reg.Add(registry.AddInput{
		Service:   service,
		Account:   account,
		Kind:      config.KindMCPStdio,
		Command:   []string{fakeMCPPath},
		SecretEnv: map[string]string{"FAKE_ACCOUNT": identity},
	})
	if err != nil {
		t.Fatalf("adding %s/%s: %v", service, account, err)
	}
}

func TestSearchFindsToolsByWordsInTheirNameOrDescriptionAcrossServices(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "one", "a", "ana")
	f.addFake(t, "two", "a", "bob")
	ctx := context.Background()

	hits, total, err := f.core.Search(ctx, "sum", "", 0)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}
	if total != 2 || len(hits) != 2 {
		t.Fatalf("got %d hits with total %d, want 2 and 2", len(hits), total)
	}
	if hits[0].Service != "one" || hits[0].Tool != "add" {
		t.Fatalf("first hit = %s/%s, want one/add", hits[0].Service, hits[0].Tool)
	}
	if hits[1].Service != "two" || hits[1].Tool != "add" {
		t.Fatalf("second hit = %s/%s, want two/add", hits[1].Service, hits[1].Tool)
	}

	hits, total, err = f.core.Search(ctx, "two numbers", "", 0)
	if err != nil {
		t.Fatalf("searching by description: %v", err)
	}
	if total != 2 || len(hits) != 2 {
		t.Fatalf("description search got %d hits with total %d, want 2 and 2", len(hits), total)
	}

	hits, total, err = f.core.Search(ctx, "SUM", "one", 1)
	if err != nil {
		t.Fatalf("searching one service: %v", err)
	}
	if total != 1 || len(hits) != 1 || hits[0].Service != "one" {
		t.Fatalf("service search got %+v with total %d, want one hit in one", hits, total)
	}
}

func TestToolsComeFromTheCacheUntilARefreshIsRequested(t *testing.T) {
	f := newFixture(t)
	f.addFake(t, "fake", "a", "ana")
	ctx := context.Background()

	catalog, err := f.core.Tools(ctx, "fake", true)
	if err != nil {
		t.Fatalf("loading tools: %v", err)
	}
	if len(catalog.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(catalog.Tools))
	}
	if _, err := os.Stat(f.store.Paths().ToolCacheFile("fake")); err != nil {
		t.Fatalf("tool cache was not written: %v", err)
	}

	if err := f.core.Close(); err != nil {
		t.Fatalf("closing core: %v", err)
	}
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Services["fake"].Command = []string{"/nonexistent/pandora-fakemcp"}
		return nil
	}); err != nil {
		t.Fatalf("changing service command: %v", err)
	}

	fresh := core.New(core.Options{
		Store:      f.store,
		Secrets:    f.secrets,
		Openers:    f.openers,
		Version:    "test",
		InstallEnv: f.installEnv,
	})
	t.Cleanup(func() {
		_ = fresh.Close()
	})

	cached, err := fresh.Tools(ctx, "fake", false)
	if err != nil {
		t.Fatalf("loading cached tools: %v", err)
	}
	if len(cached.Tools) != 4 {
		t.Fatalf("cached tools = %d, want 4", len(cached.Tools))
	}

	fallback, err := fresh.Tools(ctx, "fake", true)
	if err != nil {
		t.Fatalf("refreshing with a dead command: %v", err)
	}
	if len(fallback.Tools) != 4 {
		t.Fatalf("fallback tools = %d, want 4", len(fallback.Tools))
	}
}

func TestADeadUpstreamConnectionIsReplacedOnTheNextCall(t *testing.T) {
	f := newFixture(t)

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "upstream.pid")
	script := filepath.Join(dir, "upstream.sh")
	content := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec %q\n", pidFile, fakeMCPPath)
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatalf("writing wrapper script: %v", err)
	}

	err := f.reg.Add(registry.AddInput{
		Service:   "fake",
		Account:   "a",
		Kind:      config.KindMCPStdio,
		Command:   []string{script},
		SecretEnv: map[string]string{"FAKE_ACCOUNT": "ana"},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}
	ctx := context.Background()
	assertCallIdentity(t, f.core, ctx, "ana")

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("reading upstream pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("parsing upstream pid: %v", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("finding upstream process: %v", err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("killing upstream: %v", err)
	}

	deadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = f.core.Call(deadCtx, "fake/a", "whoami", nil)
	cancel()
	if err == nil {
		t.Fatal("calling a dead upstream succeeded, want an error")
	}

	assertCallIdentity(t, f.core, ctx, "ana")
}

func TestAccountsOpenInParallel(t *testing.T) {
	f := newFixture(t)
	err := f.reg.Add(registry.AddInput{
		Service: "fake",
		Account: "a",
		Kind:    config.KindMCPStdio,
		Command: []string{fakeMCPPath},
		Env:     map[string]string{"FAKE_START_DELAY_MS": "300"},
	})
	if err != nil {
		t.Fatalf("adding first account: %v", err)
	}
	err = f.reg.Add(registry.AddInput{
		Service: "fake",
		Account: "b",
		Env:     map[string]string{"FAKE_START_DELAY_MS": "300"},
	})
	if err != nil {
		t.Fatalf("adding second account: %v", err)
	}

	start := time.Now()
	statuses := f.core.StatusAll(context.Background())
	elapsed := time.Since(start)
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2", len(statuses))
	}
	for _, status := range statuses {
		if !status.OK {
			t.Fatalf("status = %+v, want ok", status)
		}
	}
	if elapsed >= 550*time.Millisecond {
		t.Fatalf("opening two accounts took %v, want parallel opens", elapsed)
	}
}

func assertCallIdentity(t *testing.T, app *core.Core, ctx context.Context, want string) {
	t.Helper()
	result, err := app.Call(ctx, "fake/a", "whoami", nil)
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
