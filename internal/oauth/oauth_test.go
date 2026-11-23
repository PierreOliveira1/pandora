package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakeauth"
)

func TestLoggingInStoresATokenThatAuthorizesRequestsToTheMCPServer(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	manager, out := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	port := freePort(t)

	if err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !manager.HasToken("todoist", "alice") {
		t.Error("HasToken() = false, want true")
	}
	if output := out.String(); !strings.Contains(output, "Open this URL to log in to todoist/alice:") {
		t.Errorf("output = %q, want the login URL announcement", output)
	}

	status, body := get(t, transportClient(manager, "todoist", "alice"), mcp.URL)
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if body != "alice" {
		t.Errorf("GET body = %q, want alice", body)
	}
}

func TestTheClientIsRegisteredOnceAndReusedByTheNextAccountOfTheSameService(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	port := freePort(t)

	alice, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	if err := loginWithTimeout(t, alice, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("first Login() error = %v", err)
	}
	bob, _ := newTestManager(t, store, paths, approvingBrowser(t, "bob"), nil)
	if err := loginWithTimeout(t, bob, LoginParams{
		Service:      "todoist",
		Account:      "bob",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("second Login() error = %v", err)
	}

	if registrations := auth.RegistrationCount(); registrations != 1 {
		t.Errorf("RegistrationCount() = %d, want 1", registrations)
	}
	if _, body := get(t, transportClient(alice, "todoist", "alice"), mcp.URL); body != "alice" {
		t.Errorf("alice request body = %q, want alice", body)
	}
	if _, body := get(t, transportClient(bob, "todoist", "bob"), mcp.URL); body != "bob" {
		t.Errorf("bob request body = %q, want bob", body)
	}
}

func TestAnExpiredTokenIsRefreshedBeforeTheRequest(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	clock := &manualClock{value: time.Now()}
	manager, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), clock.Now)
	port := freePort(t)

	if err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	clock.Advance(2 * time.Hour)

	status, body := get(t, transportClient(manager, "todoist", "alice"), mcp.URL)
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if body != "alice" {
		t.Errorf("GET body = %q, want alice", body)
	}
	if refreshes := auth.RefreshCount(); refreshes != 1 {
		t.Errorf("RefreshCount() = %d, want 1", refreshes)
	}
}

func TestConcurrentRefreshesFromSeparateManagersUseTheRefreshTokenOnlyOnce(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := newBarrierStore(secrets.NewMemoryStore())
	paths := testPaths(t)
	port := freePort(t)

	owner, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	if err := loginWithTimeout(t, owner, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	first, _ := newTestManager(t, store, paths, nil, nil)
	second, _ := newTestManager(t, store, paths, nil, nil)

	type refreshResult struct {
		token string
		err   error
	}
	results := make(chan refreshResult, 2)
	start := make(chan struct{})
	refresh := func(manager *Manager) {
		<-start
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		token, err := manager.ForceRefresh(ctx, "todoist", "alice")
		results <- refreshResult{token: token, err: err}
	}
	go refresh(first)
	go refresh(second)
	close(start)

	one := <-results
	two := <-results
	if one.err != nil {
		t.Fatalf("first ForceRefresh() error = %v", one.err)
	}
	if two.err != nil {
		t.Fatalf("second ForceRefresh() error = %v", two.err)
	}
	if one.token == "" || two.token == "" {
		t.Error("ForceRefresh() returned an empty token")
	}
	if one.token != two.token {
		t.Error("managers received different tokens")
	}
	if refreshes := auth.RefreshCount(); refreshes != 1 {
		t.Errorf("RefreshCount() = %d, want 1", refreshes)
	}
}

func TestARejectedRefreshTokenAsksForANewLogin(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	manager, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	port := freePort(t)

	if err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	record := storedRecord(t, store, "todoist", "alice")
	if record.Token == nil || record.Token.RefreshToken == "" {
		t.Fatal("stored token has no refresh token")
	}
	response, err := http.PostForm(record.TokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {record.Token.RefreshToken},
		"client_id":     {record.ClientID},
	})
	if err != nil {
		t.Fatalf("burning the refresh token: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("burning the refresh token: status %d", response.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := manager.ForceRefresh(ctx, "todoist", "alice"); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("ForceRefresh() error = %v, want ErrLoginRequired", err)
	}
}

func TestAServerWithoutRegistrationAsksForAClientIDAndAGivenClientIDWorks(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{DisableRegistration: true, StaticClientID: "static-client"})
	mcp := newProtectedMCPServer(t, auth, accountHandler())
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	manager, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	port := freePort(t)

	err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	})
	if err == nil {
		t.Fatal("Login() without a client ID succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "does not support automatic client registration") {
		t.Errorf("Login() error = %v, want the missing registration hint", err)
	}
	if redirect := fmt.Sprintf("http://localhost:%d/callback", port); !strings.Contains(err.Error(), redirect) {
		t.Errorf("Login() error = %v, want the redirect URL %s", err, redirect)
	}

	if err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		ClientID:     "static-client",
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() with a client ID error = %v", err)
	}
	if !manager.HasToken("todoist", "alice") {
		t.Error("HasToken() = false, want true")
	}
	if _, body := get(t, transportClient(manager, "todoist", "alice"), mcp.URL); body != "alice" {
		t.Errorf("GET body = %q, want alice", body)
	}
}

func TestA401WithAStillValidTokenTriggersOneRefreshAndARetry(t *testing.T) {
	auth := fakeauth.New(t, fakeauth.Options{})
	var calls atomic.Int64
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, "%s", r.Header.Get("X-Account"))
	})
	mcp := newProtectedMCPServer(t, auth, protected)
	store := secrets.NewMemoryStore()
	paths := testPaths(t)
	manager, _ := newTestManager(t, store, paths, approvingBrowser(t, "alice"), nil)
	port := freePort(t)

	if err := loginWithTimeout(t, manager, LoginParams{
		Service:      "todoist",
		Account:      "alice",
		MCPURL:       mcp.URL,
		RedirectPort: port,
	}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	status, body := get(t, transportClient(manager, "todoist", "alice"), mcp.URL)
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if body != "alice" {
		t.Errorf("GET body = %q, want alice", body)
	}
	if count := calls.Load(); count != 2 {
		t.Errorf("protected handler calls = %d, want 2", count)
	}
	if refreshes := auth.RefreshCount(); refreshes != 1 {
		t.Errorf("RefreshCount() = %d, want 1", refreshes)
	}
}

func newProtectedMCPServer(t *testing.T, auth *fakeauth.Server, next http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(auth.MCPHandler(next))
	t.Cleanup(server.Close)
	return server
}

func accountHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s", r.Header.Get("X-Account"))
	})
}

func newTestManager(t *testing.T, store secrets.Store, paths config.Paths, browser func(string) error, now func() time.Time) (*Manager, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	manager := NewManager(Options{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		Secrets:     store,
		Paths:       paths,
		OpenBrowser: browser,
		Out:         out,
		Now:         now,
	})
	return manager, out
}

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	return config.Paths{
		ConfigDir: t.TempDir(),
		CacheDir:  t.TempDir(),
		StateDir:  t.TempDir(),
	}
}

func approvingBrowser(t *testing.T, loginHint string) func(string) error {
	t.Helper()
	return func(rawURL string) error {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		query := parsed.Query()
		if loginHint != "" {
			query.Set("login_hint", loginHint)
		}
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
}

func transportClient(manager *Manager, service, account string) *http.Client {
	return &http.Client{
		Transport: NewTransport(http.DefaultTransport,
			func(ctx context.Context) (string, error) {
				return manager.AccessToken(ctx, service, account)
			},
			func(ctx context.Context) (string, error) {
				return manager.ForceRefresh(ctx, service, account)
			}),
		Timeout: 10 * time.Second,
	}
}

func loginWithTimeout(t *testing.T, manager *Manager, params LoginParams) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return manager.Login(ctx, params)
}

func get(t *testing.T, client *http.Client, endpoint string) (int, string) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s error = %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading GET %s: %v", endpoint, err)
	}
	return response.StatusCode, string(body)
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

func storedRecord(t *testing.T, store secrets.Store, service, account string) *Record {
	t.Helper()
	value, err := store.Get(secrets.OAuthTokenKey(service, account))
	if err != nil {
		t.Fatalf("reading stored token: %v", err)
	}
	var record Record
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		t.Fatalf("decoding stored token: %v", err)
	}
	return &record
}

type manualClock struct {
	mu    sync.Mutex
	value time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = c.value.Add(d)
}

type barrierStore struct {
	secrets.Store
	mu        sync.Mutex
	tokenGets int
	ready     chan struct{}
}

func newBarrierStore(inner secrets.Store) *barrierStore {
	return &barrierStore{Store: inner, ready: make(chan struct{})}
}

func (s *barrierStore) Get(key string) (string, error) {
	if strings.HasSuffix(key, "/oauth/token") {
		s.mu.Lock()
		s.tokenGets++
		wait := s.tokenGets <= 2
		if s.tokenGets == 2 {
			close(s.ready)
		}
		s.mu.Unlock()
		if wait {
			<-s.ready
		}
	}
	return s.Store.Get(key)
}
