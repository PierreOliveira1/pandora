package daemon_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-daemon-test-")
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
	paths    config.Paths
	store    *config.Store
	secrets  secrets.Store
	core     *core.Core
	registry *registry.Registry
	client   *daemon.Client
	cancel   context.CancelFunc
	done     chan struct{}
	errMu    sync.Mutex
	serveErr error
}

func newFixture(t *testing.T, idle time.Duration) *fixture {
	t.Helper()
	root, err := os.MkdirTemp("", "pd")
	if err != nil {
		t.Fatalf("creating test directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	paths := config.Paths{
		ConfigDir: filepath.Join(root, "c"),
		CacheDir:  filepath.Join(root, "k"),
		StateDir:  root,
	}
	store := config.NewStore(paths)
	sec := secrets.NewMemoryStore()
	openers := source.Openers{}
	mcpsource.Register(openers)
	app := core.New(core.Options{
		Store:   store,
		Secrets: sec,
		Openers: openers,
		Version: "test",
	})
	f := &fixture{
		paths:    paths,
		store:    store,
		secrets:  sec,
		core:     app,
		registry: registry.New(store, sec),
		done:     make(chan struct{}),
	}
	server := daemon.NewServer(app, "test", idle)
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	t.Cleanup(func() {
		f.close(t)
	})
	go func() {
		err := server.Serve(ctx, paths.DaemonSocket())
		f.errMu.Lock()
		f.serveErr = err
		f.errMu.Unlock()
		close(f.done)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		client, err := daemon.Dial(ctx, paths.DaemonSocket())
		if err == nil {
			f.client = client
			break
		}
		select {
		case <-f.done:
			t.Fatalf("daemon stopped before starting: %v", f.error())
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon did not start: %v", err)
		}
	}

	return f
}

func (f *fixture) close(t *testing.T) {
	t.Helper()
	if f.client != nil {
		_ = f.client.Shutdown(context.Background())
	}
	f.cancel()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Errorf("daemon did not stop")
	}
	_ = f.core.Close()
}

func (f *fixture) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop")
	}
	return f.error()
}

func (f *fixture) error() error {
	f.errMu.Lock()
	defer f.errMu.Unlock()
	return f.serveErr
}

func TestCallsThroughTheDaemonReuseOneUpstreamProcess(t *testing.T) {
	f := newFixture(t, 5*time.Minute)
	startLog := filepath.Join(f.paths.StateDir, "starts.log")
	err := f.registry.Add(registry.AddInput{
		Service: "fake",
		Account: "a",
		Kind:    config.KindMCPStdio,
		Command: []string{fakeMCPPath},
		Env: map[string]string{
			"FAKE_ACCOUNT":   "ana",
			"FAKE_START_LOG": startLog,
		},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}

	assertWhoami(t, f.client, "fake/a", "ana")
	assertWhoami(t, f.client, "fake/a", "ana")

	data, err := os.ReadFile(startLog)
	if err != nil {
		t.Fatalf("reading starts log: %v", err)
	}
	if lines := countLines(string(data)); lines != 1 {
		t.Fatalf("upstream started %d times, want 1", lines)
	}
}

func TestAChangedAccountIsPickedUpWithoutRestartingTheDaemon(t *testing.T) {
	f := newFixture(t, 5*time.Minute)
	err := f.registry.Add(registry.AddInput{
		Service:   "fake",
		Account:   "a",
		Kind:      config.KindMCPStdio,
		Command:   []string{fakeMCPPath},
		SecretEnv: map[string]string{"FAKE_ACCOUNT": "ana"},
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}
	assertWhoami(t, f.client, "fake/a", "ana")

	if err := f.registry.RemoveAccount(registry.Ref{Service: "fake", Account: "a"}); err != nil {
		t.Fatalf("removing account: %v", err)
	}
	err = f.registry.Add(registry.AddInput{
		Service:   "fake",
		Account:   "a",
		SecretEnv: map[string]string{"FAKE_ACCOUNT": "bia"},
	})
	if err != nil {
		t.Fatalf("re-adding account: %v", err)
	}

	assertWhoami(t, f.client, "fake/a", "bia")
}

func TestTheDaemonStopsByItselfAfterBeingIdle(t *testing.T) {
	f := newFixture(t, 300*time.Millisecond)
	if err := f.wait(t); err != nil {
		t.Fatalf("daemon stopped with error: %v", err)
	}
	if _, err := os.Stat(f.paths.DaemonSocket()); !os.IsNotExist(err) {
		t.Fatalf("daemon socket still exists: %v", err)
	}
}

func TestAnAccountThatNeedsLoginIsReportedTheSameWayThroughTheDaemon(t *testing.T) {
	f := newFixture(t, 5*time.Minute)
	err := f.registry.Add(registry.AddInput{
		Service: "remote",
		Account: "a",
		Kind:    config.KindMCPHTTP,
		URL:     "https://example.invalid/mcp",
		Auth:    config.AuthOAuth,
	})
	if err != nil {
		t.Fatalf("adding account: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = f.client.Call(ctx, "remote/a", "whoami", nil)
	if err == nil {
		t.Fatal("call succeeded, want a login error")
	}
	var authErr *source.AuthRequiredError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %v, want *source.AuthRequiredError", err)
	}
	if authErr.Ref != "remote/a" {
		t.Fatalf("ref = %q, want %q", authErr.Ref, "remote/a")
	}
}

func TestTheDaemonSocketIsOnlyReachableByItsOwner(t *testing.T) {
	f := newFixture(t, 5*time.Minute)
	socketPath := f.paths.DaemonSocket()
	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if socketInfo.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions = %o, want 600", socketInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		t.Fatalf("stat socket directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("socket directory permissions = %o, want 700", dirInfo.Mode().Perm())
	}
}

func assertWhoami(t *testing.T, client *daemon.Client, ref, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := client.Call(ctx, ref, "whoami", nil)
	if err != nil {
		t.Fatalf("calling whoami on %s: %v", ref, err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if text.Text != want {
		t.Fatalf("whoami on %s = %q, want %q", ref, text.Text, want)
	}
}

func countLines(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func TestASecondDaemonDoesNotStartWhileOneIsRunning(t *testing.T) {
	f := newFixture(t, 5*time.Minute)
	second := daemon.NewServer(f.core, "test", 5*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := second.Serve(ctx, f.paths.DaemonSocket()); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second daemon error = %v, want already running", err)
	}
	if _, err := f.client.Health(ctx); err != nil {
		t.Fatalf("the running daemon stopped answering: %v", err)
	}
}
