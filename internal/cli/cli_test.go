package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/cli"
	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

var fakeMCPPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pandora-cli-test-")
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

func TestAddThenListShowsTheAccountWithoutLeakingItsSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PANDORA_HOME", home)
	t.Setenv("PANDORA_SECRETS", "file")
	t.Setenv("PANDORA_NO_DAEMON", "1")

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), []string{
		"add", "gh", "--as", "personal", "--secret-env", "TOKEN=abc", "--", fakeMCPPath,
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add exited with %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Added gh/personal (mcp-stdio, default)") {
		t.Fatalf("add output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.Execute(context.Background(), []string{"list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list exited with %d: %s", code, stderr.String())
	}

	var listed struct {
		Accounts      []registry.AccountInfo `json:"accounts"`
		EmptyServices []string               `json:"empty_services"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decoding list output %q: %v", stdout.String(), err)
	}
	if len(listed.Accounts) != 1 {
		t.Fatalf("got %d accounts, want 1: %s", len(listed.Accounts), stdout.String())
	}
	account := listed.Accounts[0]
	if account.Service != "gh" || account.Account != "personal" {
		t.Fatalf("account = %s/%s, want gh/personal", account.Service, account.Account)
	}
	if account.Kind != config.KindMCPStdio {
		t.Fatalf("kind = %q, want %q", account.Kind, config.KindMCPStdio)
	}
	if account.Auth != config.AuthNone {
		t.Fatalf("auth = %q, want %q", account.Auth, config.AuthNone)
	}
	if !account.Default {
		t.Fatal("gh/personal is not the default")
	}
	if len(listed.EmptyServices) != 0 {
		t.Fatalf("empty services = %v, want none", listed.EmptyServices)
	}

	data, err := os.ReadFile(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	if strings.Contains(string(data), "abc") {
		t.Fatal("config.toml contains the secret value")
	}
}

func TestUserAddsALocalMCPAndCallsATool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PANDORA_HOME", home)
	t.Setenv("PANDORA_SECRETS", "file")
	t.Setenv("PANDORA_NO_DAEMON", "1")

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), []string{
		"add", "fake", "--as", "a", "--secret-env", "FAKE_ACCOUNT=ana", "--", fakeMCPPath,
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add exited with %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Connected: 4 tools available") {
		t.Fatalf("add output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.Execute(context.Background(), []string{"call", "fake/a", "whoami"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("call exited with %d: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "ana" {
		t.Fatalf("call output = %q, want %q", got, "ana")
	}
}

func TestAddingAnAccountUpdatesTheSkillOfEveryInstalledAIClient(t *testing.T) {
	home, pandoraHome := installedCLIHomes(t)
	t.Setenv("HOME", home)
	t.Setenv("PANDORA_HOME", pandoraHome)
	t.Setenv("PANDORA_SECRETS", "file")
	t.Setenv("PANDORA_NO_DAEMON", "1")

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), []string{"install"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install exited with %d: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.Execute(context.Background(), []string{
		"add", "fake", "--as", "b", "--secret-env", "FAKE_ACCOUNT=ana", "--", fakeMCPPath,
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add exited with %d: %s", code, stderr.String())
	}

	paths := []string{
		filepath.Join(home, ".claude", "skills", "pandora", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "pandora", "SKILL.md"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if !strings.Contains(string(data), "fake (accounts: b)") {
			t.Fatalf("%s = %q, want the new account", path, data)
		}
	}
}

func TestUninstallingRemovesTheSkillAndThePermission(t *testing.T) {
	home, pandoraHome := installedCLIHomes(t)
	t.Setenv("HOME", home)
	t.Setenv("PANDORA_HOME", pandoraHome)
	t.Setenv("PANDORA_SECRETS", "file")
	t.Setenv("PANDORA_NO_DAEMON", "1")

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), []string{"install"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install exited with %d: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = cli.Execute(context.Background(), []string{"uninstall"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("uninstall exited with %d: %s", code, stderr.String())
	}

	for _, path := range []string{
		filepath.Join(home, ".claude", "skills", "pandora"),
		filepath.Join(home, ".config", "opencode", "skills", "pandora"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("skill at %s still exists: %v", path, err)
		}
	}

	settings, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading claude settings: %v", err)
	}
	if strings.Contains(string(settings), "Bash(pandora:*)") {
		t.Fatalf("claude settings still allow pandora: %s", settings)
	}
	openCode, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatalf("reading opencode config: %v", err)
	}
	if strings.Contains(string(openCode), "pandora *") {
		t.Fatalf("opencode config still allows pandora: %s", openCode)
	}

	paths, err := config.DefaultPaths(os.Getenv)
	if err != nil {
		t.Fatalf("finding paths: %v", err)
	}
	cfg, err := config.NewStore(paths).Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Integrations) != 0 {
		t.Fatalf("Integrations = %v, want none", cfg.Integrations)
	}
}

func installedCLIHomes(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	pandoraHome := t.TempDir()
	for _, dir := range []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "opencode"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}
	return home, pandoraHome
}
