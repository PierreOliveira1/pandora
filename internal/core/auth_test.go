package core_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/oauth"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakeauth"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

type authSetup struct {
	store    *config.Store
	secrets  secrets.Store
	paths    config.Paths
	manager  *oauth.Manager
	core     *core.Core
	registry *registry.Registry
	hint     string
}

func newAuthSetup(t *testing.T) *authSetup {
	t.Helper()
	root := t.TempDir()
	paths := config.Paths{
		ConfigDir: filepath.Join(root, "config"),
		CacheDir:  filepath.Join(root, "cache"),
		StateDir:  filepath.Join(root, "state"),
	}
	store := config.NewStore(paths)
	sec := secrets.NewMemoryStore()
	setup := &authSetup{
		store:    store,
		secrets:  sec,
		paths:    paths,
		registry: registry.New(store, sec),
	}
	manager := oauth.NewManager(oauth.Options{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		Secrets:     sec,
		Paths:       paths,
		OpenBrowser: func(rawURL string) error { return approveLogin(t, rawURL, setup.hint) },
		Out:         io.Discard,
	})
	openers := source.Openers{}
	mcpsource.Register(openers)
	setup.manager = manager
	setup.core = core.New(core.Options{
		Store:   store,
		Secrets: sec,
		Openers: openers,
		Version: "test",
		OAuth:   manager,
	})
	t.Cleanup(func() {
		_ = setup.core.Close()
	})
	return setup
}

func (s *authSetup) addService(t *testing.T, mcpURL string, mode config.AuthMode, redirectPort int) {
	t.Helper()
	err := s.registry.Add(registry.AddInput{
		Service: "todoist",
		Account: "ana",
		Kind:    config.KindMCPHTTP,
		URL:     mcpURL,
		Auth:    mode,
	})
	if err != nil {
		t.Fatalf("adding service: %v", err)
	}
	if redirectPort == 0 {
		return
	}
	err = s.store.Update(func(cfg *config.Config) error {
		service := cfg.Services["todoist"]
		if service.OAuth == nil {
			service.OAuth = &config.OAuthConfig{}
		}
		service.OAuth.RedirectPort = redirectPort
		return nil
	})
	if err != nil {
		t.Fatalf("setting redirect port: %v", err)
	}
}

func (s *authSetup) login(t *testing.T, hint, ref string) {
	t.Helper()
	s.hint = hint
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.core.Login(ctx, ref); err != nil {
		t.Fatalf("logging in %s: %v", ref, err)
	}
}

func TestEachOAuthAccountLogsInOnceAndThenCallsToolsAsItsOwnUser(t *testing.T) {
	setup := newAuthSetup(t)
	authServer, mcp := newOAuthMCPServer(t, fakeauth.Options{})
	port := freePort(t)
	setup.addService(t, mcp.URL, config.AuthOAuth, port)
	if err := setup.registry.Add(registry.AddInput{Service: "todoist", Account: "bia"}); err != nil {
		t.Fatalf("adding second account: %v", err)
	}

	setup.login(t, "ana", "todoist/ana")
	setup.login(t, "bia", "todoist/bia")

	if got := authServer.RegistrationCount(); got != 1 {
		t.Fatalf("RegistrationCount() = %d, want 1", got)
	}
	assertWhoami(t, setup.core, "todoist/ana", "ana")
	assertWhoami(t, setup.core, "todoist/bia", "bia")
}

func TestAServerThatAsksForLoginTurnsTheServiceIntoAnOAuthServiceWhenTheAccountIsAdded(t *testing.T) {
	setup := newAuthSetup(t)
	_, mcp := newOAuthMCPServer(t, fakeauth.Options{})
	setup.addService(t, mcp.URL, config.AuthNone, 0)

	info, err := setup.core.DetectAuth(context.Background(), "todoist/ana")
	if err != nil {
		t.Fatalf("detecting auth: %v", err)
	}
	if !info.NeedsLogin {
		t.Fatalf("status = %+v, want needs login", info)
	}

	cfg, err := setup.store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	service := cfg.Services["todoist"]
	if service.Auth != config.AuthOAuth {
		t.Fatalf("auth = %q, want %q", service.Auth, config.AuthOAuth)
	}
	if service.OAuth == nil {
		t.Fatal("OAuth config is nil")
	}
}

func TestLoggingOutMakesTheAccountNeedLoginAgain(t *testing.T) {
	setup := newAuthSetup(t)
	_, mcp := newOAuthMCPServer(t, fakeauth.Options{})
	port := freePort(t)
	setup.addService(t, mcp.URL, config.AuthOAuth, port)
	setup.login(t, "ana", "todoist/ana")

	if !setup.manager.HasToken("todoist", "ana") {
		t.Fatal("token is missing after login")
	}
	if err := setup.core.Logout(context.Background(), "todoist/ana"); err != nil {
		t.Fatalf("logging out: %v", err)
	}
	if setup.manager.HasToken("todoist", "ana") {
		t.Fatal("token still exists after logout")
	}
	status := setup.core.Status(context.Background(), "todoist/ana")
	if !status.NeedsLogin {
		t.Fatalf("status = %+v, want needs login", status)
	}
}

func TestAnExpiredTokenIsRenewedDuringACallWithoutANewLogin(t *testing.T) {
	setup := newAuthSetup(t)
	authServer, mcp := newOAuthMCPServer(t, fakeauth.Options{AccessTTL: 62 * time.Second})
	port := freePort(t)
	setup.addService(t, mcp.URL, config.AuthOAuth, port)
	setup.login(t, "ana", "todoist/ana")

	time.Sleep(3 * time.Second)
	assertWhoami(t, setup.core, "todoist/ana", "ana")

	if got := authServer.RefreshCount(); got != 1 {
		t.Fatalf("RefreshCount() = %d, want 1", got)
	}
}

func newOAuthMCPServer(t *testing.T, options fakeauth.Options) (*fakeauth.Server, *httptest.Server) {
	t.Helper()
	auth := fakeauth.New(t, options)
	server := httptest.NewServer(auth.MCPHandler(fakemcp.NewHTTPHandler("")))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	return auth, server
}

func approveLogin(t *testing.T, rawURL, hint string) error {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	query := parsed.Query()
	query.Set("login_hint", hint)
	parsed.RawQuery = query.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(parsed.String())
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("login page returned %s: %s", response.Status, body)
	}
	return nil
}

func assertWhoami(t *testing.T, app *core.Core, ref, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := app.Call(ctx, ref, "whoami", nil)
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

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
