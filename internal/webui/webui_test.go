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
	"testing"
	"testing/fstest"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
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
	ts     *httptest.Server
	client *http.Client
	store  *config.Store
	paths  config.Paths
	port   string
}

func newUIFixture(t *testing.T) *uiFixture {
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
	ui := New(Options{
		Core:     app,
		Registry: reg,
		CallLog:  guard.NewLog(paths.CallLog()),
		Version:  "test",
		Token:    "test-token",
		Static:   fstest.MapFS{"index.html": {Data: []byte(testIndexPage)}},
	})

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
		ts:     ts,
		client: &http.Client{Jar: jar, Timeout: 20 * time.Second},
		store:  store,
		paths:  paths,
		port:   port,
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
