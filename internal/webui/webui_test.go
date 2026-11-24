package webui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/events"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/command"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-webui-test-")
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

const testIndexPage = `<!doctype html><p>Pandora test page</p><script>start()</script><script type="module" src="/_app/start.js"></script>`

func TestTheAppPageMayRunOnlyItsOwnStartupScript(t *testing.T) {
	f := newUIFixture(t)
	response, err := f.client.Get(f.ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	response.Body.Close()
	sum := sha256.Sum256([]byte("start()"))
	want := "script-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"
	if policy := response.Header.Get("Content-Security-Policy"); !strings.Contains(policy, want) {
		t.Fatalf("Content-Security-Policy = %q, want it to contain %q", policy, want)
	}
}

type uiFixture struct {
	ts      *httptest.Server
	client  *http.Client
	store   *config.Store
	secrets secrets.Store
	core    *core.Core
	paths   config.Paths
	port    string
}

func newUIFixture(t *testing.T) *uiFixture {
	return newConfiguredUIFixture(t, nil)
}

func newConfiguredUIFixture(t *testing.T, configure func(*Options)) *uiFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PANDORA_HOME", home)
	paths, err := config.DefaultPaths(os.Getenv)
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}
	store := config.NewStore(paths)
	sec := secrets.NewFileStore(paths.SecretsFile())
	openers := source.Openers{}
	mcpsource.Register(openers)
	command.Register(openers)
	installEnv := install.Env{
		Home:     t.TempDir(),
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}
	app := core.New(core.Options{
		Store:      store,
		Secrets:    sec,
		Openers:    openers,
		Version:    "test",
		InstallEnv: installEnv,
	})
	reg := registry.New(store, sec)
	options := Options{
		Core:     app,
		Registry: reg,
		CallLog:  guard.NewLog(paths.CallLog()),
		Version:  "test",
		Token:    "test-token",
		Static:   fstest.MapFS{"index.html": {Data: []byte(testIndexPage)}},
	}
	if configure != nil {
		configure(&options)
	}
	ui := New(options)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	ts := httptest.NewUnstartedServer(nil)
	_ = ts.Listener.Close()
	ts.Listener = listener
	ts.Config.Handler = ui.Handler(listener.Addr().String())
	ts.Start()
	t.Cleanup(func() {
		ts.Close()
		_ = app.Close()
	})
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return &uiFixture{
		ts:      ts,
		client:  &http.Client{Jar: jar, Timeout: 20 * time.Second},
		store:   store,
		secrets: sec,
		core:    app,
		paths:   paths,
		port:    port,
	}
}

func TestTheAPIRefusesRequestsWithoutTheSession(t *testing.T) {
	f := newUIFixture(t)
	status, body := f.do(t, http.MethodGet, "/api/overview", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if !strings.Contains(body, "open the link printed by pandora ui") {
		t.Errorf("body = %q, want the session hint", body)
	}
}

func TestOpeningTheSessionLinkSetsTheCookieAndUnlocksTheAPI(t *testing.T) {
	f := newUIFixture(t)
	status, body := f.do(t, http.MethodGet, "/?token=test-token", "", nil)
	if status != http.StatusOK {
		t.Fatalf("session status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, "Pandora test page") {
		t.Errorf("session body = %q, want the app page", body)
	}

	status, body = f.do(t, http.MethodGet, "/api/overview", "", nil)
	if status != http.StatusOK {
		t.Fatalf("overview status = %d, want 200: %s", status, body)
	}
}

func TestAnotherSiteCannotChangeAnything(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	body := `{"service":"demo","account":"main","kind":"command"}`

	status, _ := f.do(t, http.MethodPost, "/api/accounts", body, map[string]string{
		"X-Pandora": "1",
		"Origin":    "http://evil.example",
	})
	if status != http.StatusForbidden {
		t.Errorf("foreign Origin status = %d, want 403", status)
	}

	status, _ = f.do(t, http.MethodPost, "/api/accounts", body, map[string]string{
		"Origin": f.ts.URL,
	})
	if status != http.StatusForbidden {
		t.Errorf("missing X-Pandora status = %d, want 403", status)
	}
}

func TestAForeignHostHeaderIsRefused(t *testing.T) {
	f := newUIFixture(t)
	request, err := http.NewRequest(http.MethodGet, f.ts.URL+"/api/overview", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Host = "evil.example:" + f.port
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	if !strings.Contains(string(body), "forbidden host") {
		t.Errorf("body = %q, want forbidden host", body)
	}
}

func TestAServiceWithoutToolsListsAnEmptyToolList(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	status, response := f.do(t, http.MethodPost, "/api/accounts", `{"service":"ops","account":"local","kind":"command"}`, csrf())
	if status != http.StatusCreated {
		t.Fatalf("add status = %d, want 201: %s", status, response)
	}

	status, response = f.do(t, http.MethodGet, "/api/services/ops/tools", "", nil)
	if status != http.StatusOK {
		t.Fatalf("tools status = %d, want 200: %s", status, response)
	}
	if !strings.Contains(response, `"tools":[]`) {
		t.Errorf("tools response = %s, want an empty tools list", response)
	}
}

func TestAnAccountAddedThroughTheAPIShowsUpInTheOverviewAndItsSecretsNeverComeBack(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	body := `{"service":"demo","account":"main","kind":"command","service_description":"Demo service","description":"Main account","env":{"VISIBLE":"yes"},"secret_env":{"TOKEN":"s3cr3t"}}`
	status, response := f.do(t, http.MethodPost, "/api/accounts", body, csrf())
	if status != http.StatusCreated {
		t.Fatalf("add status = %d, want 201: %s", status, response)
	}
	if strings.Contains(response, "s3cr3t") {
		t.Fatal("the add response contains the secret")
	}

	status, response = f.do(t, http.MethodGet, "/api/overview", "", nil)
	if status != http.StatusOK {
		t.Fatalf("overview status = %d, want 200: %s", status, response)
	}
	if strings.Contains(response, "s3cr3t") {
		t.Fatal("the overview contains the secret")
	}
	var overview core.Overview
	if err := json.Unmarshal([]byte(response), &overview); err != nil {
		t.Fatalf("decoding overview: %v", err)
	}
	if overview.Version != "test" {
		t.Errorf("version = %q, want test", overview.Version)
	}
	if len(overview.Services) != 1 {
		t.Fatalf("services = %v, want one", overview.Services)
	}
	service := overview.Services[0]
	if service.Name != "demo" || service.Description != "Demo service" {
		t.Errorf("service = %+v, want demo", service)
	}
	if len(service.Accounts) != 1 {
		t.Fatalf("accounts = %v, want one", service.Accounts)
	}
	account := service.Accounts[0]
	if account.Name != "main" || account.Description != "Main account" || !account.Default {
		t.Errorf("account = %+v, want main and default", account)
	}

	configData, err := os.ReadFile(f.paths.ConfigFile())
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if strings.Contains(string(configData), "s3cr3t") {
		t.Fatal("the config file contains the secret")
	}

	for _, endpoint := range []string{
		"/api/status",
		"/api/accounts/demo/main/policy",
		"/api/log",
		"/api/doctor?offline=1",
	} {
		status, response = f.do(t, http.MethodGet, endpoint, "", nil)
		if status != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", endpoint, status, response)
		}
		if strings.Contains(response, "s3cr3t") {
			t.Fatalf("%s contains the secret", endpoint)
		}
	}
}

func TestAToolCanBeTestedFromTheAPI(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	add := fmt.Sprintf(`{"service":"fake","account":"a","kind":"mcp-stdio","command":[%q],"env":{"FAKE_ACCOUNT":"ana"}}`, fakeMCPPath)
	status, response := f.do(t, http.MethodPost, "/api/accounts", add, csrf())
	if status != http.StatusCreated {
		t.Fatalf("add status = %d, want 201: %s", status, response)
	}

	status, response = f.do(t, http.MethodPost, "/api/call", `{"ref":"fake/a","tool":"whoami","args":{}}`, csrf())
	if status != http.StatusOK {
		t.Fatalf("call status = %d, want 200: %s", status, response)
	}
	if !strings.Contains(response, "ana") {
		t.Errorf("call response = %q, want the account identity", response)
	}
}

func addWebhookForUITest(t *testing.T, f *uiFixture, service, account string) {
	t.Helper()
	err := f.store.Update(func(cfg *config.Config) error {
		if cfg.Services == nil {
			cfg.Services = map[string]*config.Service{}
		}
		svc := cfg.Services[service]
		if svc == nil {
			svc = &config.Service{Kind: config.KindCommand}
			cfg.Services[service] = svc
		}
		if svc.Accounts == nil {
			svc.Accounts = map[string]*config.Account{}
		}
		acc := svc.Accounts[account]
		if acc == nil {
			acc = &config.Account{}
			svc.Accounts[account] = acc
		}
		acc.Webhook = &config.Webhook{ID: "hook-1", Style: "hmac", Challenge: "hub"}
		if svc.DefaultAccount == "" {
			svc.DefaultAccount = account
		}
		return nil
	})
	if err != nil {
		t.Fatalf("adding webhook: %v", err)
	}
}

func TestTheWebhookListShowsThePublicURL(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	addWebhookForUITest(t, f, "one", "local")
	if err := f.store.Update(func(cfg *config.Config) error {
		cfg.Webhooks = &config.WebhookSettings{Port: 4321, PublicURL: "https://hooks.example.com/pandora"}
		return nil
	}); err != nil {
		t.Fatalf("saving webhook settings: %v", err)
	}
	if err := f.secrets.Set(secrets.WebhookSigningKey("one", "local"), "signing-secret"); err != nil {
		t.Fatalf("setting signing secret: %v", err)
	}
	if err := f.secrets.Set(secrets.WebhookVerifyTokenKey("one", "local"), "verify-token"); err != nil {
		t.Fatalf("setting verify token: %v", err)
	}

	status, body := f.do(t, http.MethodGet, "/api/webhooks", "", nil)
	if status != http.StatusOK {
		t.Fatalf("webhooks status = %d, want 200: %s", status, body)
	}
	var infos []core.WebhookInfo
	if err := json.Unmarshal([]byte(body), &infos); err != nil {
		t.Fatalf("decoding webhooks: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("webhooks = %+v, want one", infos)
	}
	wantURL := "https://hooks.example.com/pandora/hooks/one/local/hook-1"
	if infos[0].Ref != "one/local" || infos[0].URL != wantURL {
		t.Fatalf("webhook = %+v, want ref one/local and URL %s", infos[0], wantURL)
	}
	wantLocal := "http://127.0.0.1:4321/hooks/one/local/hook-1"
	if infos[0].LocalURL != wantLocal {
		t.Fatalf("LocalURL = %q, want %q", infos[0].LocalURL, wantLocal)
	}
	if strings.Contains(body, "signing-secret") || strings.Contains(body, "verify-token") {
		t.Fatal("the webhook list contains a secret")
	}
}

func TestRemovingAWebhookRemovesItAndReloads(t *testing.T) {
	var reloads atomic.Int32
	f := newConfiguredUIFixture(t, func(options *Options) {
		options.ReloadWebhooks = func(ctx context.Context) {
			reloads.Add(1)
		}
	})
	f.login(t)
	addWebhookForUITest(t, f, "one", "local")

	status, body := f.do(t, http.MethodDelete, "/api/accounts/one/local/webhook", "", csrf())
	if status != http.StatusOK {
		t.Fatalf("remove status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("remove body = %q, want ok", body)
	}
	if reloads.Load() != 1 {
		t.Fatalf("reload calls = %d, want 1", reloads.Load())
	}

	status, body = f.do(t, http.MethodGet, "/api/webhooks", "", nil)
	if status != http.StatusOK {
		t.Fatalf("webhooks status = %d, want 200: %s", status, body)
	}
	var infos []core.WebhookInfo
	if err := json.Unmarshal([]byte(body), &infos); err != nil {
		t.Fatalf("decoding webhooks: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("webhooks = %+v, want none", infos)
	}
	if strings.TrimSpace(body) != "[]" {
		t.Fatalf("empty webhooks body = %q, want []", body)
	}
}

func TestEventsComeNewestFirstAndRespectFilterAndLimit(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)
	ctx := context.Background()
	items := []events.Event{
		{Service: "one", Account: "local", Topic: "a"},
		{Service: "one", Account: "local", Topic: "b"},
		{Service: "two", Account: "local", Topic: "a"},
		{Service: "one", Account: "local", Topic: "a"},
		{Service: "one", Account: "other", Topic: "a"},
		{Service: "one", Account: "local", Topic: "a"},
	}
	for i := range items {
		items[i].Payload = json.RawMessage(fmt.Sprintf(`{"n":%d}`, i+1))
		if _, _, err := f.core.Events().Append(ctx, items[i], ""); err != nil {
			t.Fatalf("appending event: %v", err)
		}
	}

	status, body := f.do(t, http.MethodGet, "/api/events?service=one&account=local&topic=a&limit=2", "", nil)
	if status != http.StatusOK {
		t.Fatalf("events status = %d, want 200: %s", status, body)
	}
	var recent []events.Event
	if err := json.Unmarshal([]byte(body), &recent); err != nil {
		t.Fatalf("decoding events: %v", err)
	}
	if len(recent) != 2 || recent[0].Seq != 6 || recent[1].Seq != 4 {
		t.Fatalf("events = %+v, want seqs 6 and 4", recent)
	}
	if string(recent[0].Payload) != `{"n":6}` {
		t.Fatalf("newest payload = %s", recent[0].Payload)
	}

	for _, query := range []string{"limit=abc", "limit=501"} {
		status, body = f.do(t, http.MethodGet, "/api/events?"+query, "", nil)
		if status != http.StatusBadRequest {
			t.Fatalf("events with %s status = %d, want 400: %s", query, status, body)
		}
	}
}

func TestTheTunnelWithoutConfigurationReportsExposedFalseAndAProblem(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)

	status, body := f.do(t, http.MethodGet, "/api/tunnel", "", nil)
	if status != http.StatusOK {
		t.Fatalf("tunnel status = %d, want 200: %s", status, body)
	}
	var info core.TunnelInfo
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("decoding tunnel: %v", err)
	}
	if info.Exposed {
		t.Fatal("tunnel is exposed without configuration")
	}
	if info.Problem != "no tunnel: run pandora tunnel use tailscale" {
		t.Fatalf("Problem = %q", info.Problem)
	}
}

type fakeUIAutostartControl struct {
	enabled      bool
	enableCalls  int
	disableCalls int
}

func (f *fakeUIAutostartControl) Enable(ctx context.Context) error {
	f.enableCalls++
	f.enabled = true
	return nil
}

func (f *fakeUIAutostartControl) Disable(ctx context.Context) error {
	f.disableCalls++
	f.enabled = false
	return nil
}

func (f *fakeUIAutostartControl) Enabled(ctx context.Context) (bool, error) {
	return f.enabled, nil
}

func TestAutostartTurnsOnAndOffWithAFakeControl(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("autostart is supported only on macOS and Linux")
	}
	fake := &fakeUIAutostartControl{}
	f := newConfiguredUIFixture(t, func(options *Options) {
		options.Autostart = func() (AutostartControl, error) {
			return fake, nil
		}
	})
	f.login(t)

	status, body := f.do(t, http.MethodGet, "/api/autostart", "", nil)
	if status != http.StatusOK {
		t.Fatalf("autostart status = %d, want 200: %s", status, body)
	}
	var state struct {
		Supported bool `json:"supported"`
		Enabled   bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatalf("decoding autostart: %v", err)
	}
	if !state.Supported || state.Enabled {
		t.Fatalf("autostart = %+v, want supported and disabled", state)
	}

	status, body = f.do(t, http.MethodPut, "/api/autostart", `{"enabled":true}`, csrf())
	if status != http.StatusOK {
		t.Fatalf("enable status = %d, want 200: %s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatalf("decoding enabled autostart: %v", err)
	}
	if !state.Supported || !state.Enabled || fake.enableCalls != 1 || !fake.enabled {
		t.Fatalf("enable result = %+v, calls = %d", state, fake.enableCalls)
	}

	status, body = f.do(t, http.MethodPut, "/api/autostart", `{"enabled":false}`, csrf())
	if status != http.StatusOK {
		t.Fatalf("disable status = %d, want 200: %s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatalf("decoding disabled autostart: %v", err)
	}
	if !state.Supported || state.Enabled || fake.disableCalls != 1 || fake.enabled {
		t.Fatalf("disable result = %+v, calls = %d", state, fake.disableCalls)
	}
}

func TestAutostartWithoutAControlIsUnsupported(t *testing.T) {
	f := newUIFixture(t)
	f.login(t)

	status, body := f.do(t, http.MethodGet, "/api/autostart", "", nil)
	if status != http.StatusOK {
		t.Fatalf("autostart status = %d, want 200: %s", status, body)
	}
	var state struct {
		Supported bool `json:"supported"`
		Enabled   bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatalf("decoding autostart: %v", err)
	}
	if state.Supported || state.Enabled {
		t.Fatalf("autostart = %+v, want unsupported and disabled", state)
	}

	status, body = f.do(t, http.MethodPut, "/api/autostart", `{"enabled":true}`, csrf())
	if status != http.StatusBadRequest {
		t.Fatalf("autostart change status = %d, want 400: %s", status, body)
	}
	if !strings.Contains(body, "starting Pandora at login is not supported on this system") {
		t.Fatalf("autostart change body = %q", body)
	}
}

func (f *uiFixture) login(t *testing.T) {
	t.Helper()
	status, body := f.do(t, http.MethodGet, "/?token=test-token", "", nil)
	if status != http.StatusOK {
		t.Fatalf("session status = %d, want 200: %s", status, body)
	}
}

func (f *uiFixture) do(t *testing.T, method, path, body string, headers map[string]string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, f.ts.URL+path, reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatalf("Do(%s %s) error = %v", method, path, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return response.StatusCode, string(data)
}

func csrf() map[string]string {
	return map[string]string{"X-Pandora": "1"}
}
