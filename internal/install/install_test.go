package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testEnv(t *testing.T) Env {
	t.Helper()
	return Env{
		Home: t.TempDir(),
		GOOS: "darwin",
		LookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("unexpected command execution")
			return nil, errors.New("unconfigured runner")
		},
	}
}

func mustConfigPath(t *testing.T, env Env, c Client) string {
	t.Helper()
	path, err := ConfigPath(env, c)
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	return path
}

func writeConfig(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod(%q) error = %v", path, err)
	}
}

func readConfig(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}

func mustInstall(t *testing.T, env Env, c Client, e Entry) Result {
	t.Helper()
	res, err := Install(context.Background(), env, c, e)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	return res
}

func decodeConfig(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return config
}

func TestClaudeDesktopConfigIsFoundUnderAppDataOnWindows(t *testing.T) {
	env := testEnv(t)
	env.GOOS = "windows"

	path, err := ConfigPath(env, ClaudeDesktop)
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	want := filepath.Join(env.Home, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	if path != want {
		t.Errorf("ConfigPath() = %q, want %q", path, want)
	}
}

func TestInstallingIntoCodexKeepsTheUsersCommentsAndOtherServers(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, Codex)
	writeConfig(t, path, `# keep this comment
model = "gpt-5"

[mcp_servers.x]
command = "x-server"
args = ["--flag"]
`, 0o600)

	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}
	res := mustInstall(t, env, Codex, e)

	if res.Client != Codex || res.Method != "file" || res.ConfigPath != path {
		t.Errorf("result = %+v, want client %s, method file, path %s", res, Codex, path)
	}
	if !res.Changed {
		t.Error("Install() Changed = false, want true")
	}
	if res.BackupPath != path+".pandora.bak" {
		t.Errorf("BackupPath = %q, want %q", res.BackupPath, path+".pandora.bak")
	}

	content := string(readConfig(t, path))
	for _, want := range []string{
		"# keep this comment",
		`model = "gpt-5"`,
		"[mcp_servers.x]",
		`command = "x-server"`,
		`args = ["--flag"]`,
		"[mcp_servers.pandora]",
		`command = "/opt/pandora/bin/pandora"`,
		`args = ["serve"]`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config does not contain %q:\n%s", want, content)
		}
	}
}

func TestInstallingTwiceChangesNothingTheSecondTime(t *testing.T) {
	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}

	t.Run("codex", func(t *testing.T) {
		env := testEnv(t)
		path := mustConfigPath(t, env, Codex)
		first := mustInstall(t, env, Codex, e)
		if !first.Changed {
			t.Error("first Install() Changed = false, want true")
		}
		before := readConfig(t, path)

		second := mustInstall(t, env, Codex, e)
		if second.Changed {
			t.Error("second Install() Changed = true, want false")
		}
		if second.BackupPath != "" {
			t.Errorf("second BackupPath = %q, want empty", second.BackupPath)
		}
		if after := readConfig(t, path); !bytes.Equal(after, before) {
			t.Errorf("config changed on the second install:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("cursor", func(t *testing.T) {
		env := testEnv(t)
		path := mustConfigPath(t, env, Cursor)
		first := mustInstall(t, env, Cursor, e)
		if !first.Changed {
			t.Error("first Install() Changed = false, want true")
		}
		before := readConfig(t, path)

		second := mustInstall(t, env, Cursor, e)
		if second.Changed {
			t.Error("second Install() Changed = true, want false")
		}
		if second.BackupPath != "" {
			t.Errorf("second BackupPath = %q, want empty", second.BackupPath)
		}
		if after := readConfig(t, path); !bytes.Equal(after, before) {
			t.Errorf("config changed on the second install:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})
}

func TestReinstallingIntoCodexReplacesTheOldBlockIncludingItsSubTables(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, Codex)
	writeConfig(t, path, `# keep this comment
[mcp_servers.pandora]
command = "/old/pandora"
args = ["serve", "--old"]

[mcp_servers.pandora.env]
OLD = "value"

[mcp_servers.other]
command = "other-server"
`, 0o600)

	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}
	res := mustInstall(t, env, Codex, e)
	if !res.Changed {
		t.Error("Install() Changed = false, want true")
	}

	content := string(readConfig(t, path))
	for _, unwanted := range []string{"/old/pandora", `"--old"`, "[mcp_servers.pandora.env]", `OLD = "value"`} {
		if strings.Contains(content, unwanted) {
			t.Errorf("config still contains %q:\n%s", unwanted, content)
		}
	}
	for _, want := range []string{
		"# keep this comment",
		"[mcp_servers.pandora]",
		`command = "/opt/pandora/bin/pandora"`,
		`args = ["serve"]`,
		"[mcp_servers.other]",
		`command = "other-server"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config does not contain %q:\n%s", want, content)
		}
	}
}

func TestUninstallingFromCodexRemovesOnlyThePandoraBlock(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, Codex)
	writeConfig(t, path, `# keep this comment

[mcp_servers.other]
command = "other-server"

[mcp_servers.pandora]
command = "/opt/pandora/bin/pandora"
args = ["serve"]

[settings]
theme = "dark"
`, 0o600)

	res, err := Uninstall(context.Background(), env, Codex, "pandora")
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.Changed {
		t.Error("Uninstall() Changed = false, want true")
	}

	content := string(readConfig(t, path))
	if strings.Contains(content, "mcp_servers.pandora") {
		t.Errorf("config still contains the pandora block:\n%s", content)
	}
	for _, want := range []string{
		"# keep this comment",
		"[mcp_servers.other]",
		`command = "other-server"`,
		"[settings]",
		`theme = "dark"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config does not contain %q:\n%s", want, content)
		}
	}

	before := readConfig(t, path)
	second, err := Uninstall(context.Background(), env, Codex, "pandora")
	if err != nil {
		t.Fatalf("second Uninstall() error = %v", err)
	}
	if second.Changed {
		t.Error("second Uninstall() Changed = true, want false")
	}
	if after := readConfig(t, path); !bytes.Equal(after, before) {
		t.Errorf("config changed on the second uninstall:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestInstallingIntoOpenCodeCreatesTheConfigWhenItDoesNotExist(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, OpenCode)
	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}

	res := mustInstall(t, env, OpenCode, e)
	if res.Client != OpenCode || res.Method != "file" || res.ConfigPath != path {
		t.Errorf("result = %+v, want client %s, method file, path %s", res, OpenCode, path)
	}
	if !res.Changed {
		t.Error("Install() Changed = false, want true")
	}
	if res.BackupPath != "" {
		t.Errorf("BackupPath = %q, want empty", res.BackupPath)
	}

	data := readConfig(t, path)
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Error("config does not end with a newline")
	}
	config := decodeConfig(t, data)
	if config["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema = %v, want the OpenCode schema", config["$schema"])
	}
	servers, ok := config["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("mcp = %v, want an object", config["mcp"])
	}
	entry, ok := servers["pandora"].(map[string]any)
	if !ok {
		t.Fatalf("mcp.pandora = %v, want an object", servers["pandora"])
	}
	if entry["type"] != "local" {
		t.Errorf("type = %v, want local", entry["type"])
	}
	if entry["enabled"] != true {
		t.Errorf("enabled = %v, want true", entry["enabled"])
	}
	if want := []any{"/opt/pandora/bin/pandora", "serve"}; !reflect.DeepEqual(entry["command"], want) {
		t.Errorf("command = %v, want %v", entry["command"], want)
	}
}

func TestAnOpenCodeJSONCConfigIsNeverRewritten(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, OpenCode)
	jsoncPath := path + "c"
	original := `{
  // my settings
  "mcp": {}
}
`
	writeConfig(t, jsoncPath, original, 0o600)
	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}

	_, err := Install(context.Background(), env, OpenCode, e)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Install() error = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.Contains(err.Error(), path+" could not be parsed as JSON; add this manually:") {
		t.Errorf("error = %q, want the path and manual instructions", err)
	}
	if after := string(readConfig(t, jsoncPath)); after != original {
		t.Errorf("jsonc config was rewritten:\nbefore:\n%s\nafter:\n%s", original, after)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%q) error = %v, want file to not exist", path, err)
	}
}

func TestJSONClientsKeepTheOtherServersAndSettings(t *testing.T) {
	tests := []struct {
		name   string
		client Client
	}{
		{name: "cursor", client: Cursor},
		{name: "claude desktop", client: ClaudeDesktop},
		{name: "claude code", client: ClaudeCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testEnv(t)
			path := mustConfigPath(t, env, tt.client)
			writeConfig(t, path, `{
  "theme": "dark",
  "mcpServers": {
    "other": {
      "command": "other-server",
      "args": ["--other"]
    }
  }
}
`, 0o600)

			e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}
			res := mustInstall(t, env, tt.client, e)
			if !res.Changed {
				t.Error("Install() Changed = false, want true")
			}
			if res.Method != "file" {
				t.Errorf("Method = %q, want file", res.Method)
			}

			config := decodeConfig(t, readConfig(t, path))
			if config["theme"] != "dark" {
				t.Errorf("theme = %v, want dark", config["theme"])
			}
			servers, ok := config["mcpServers"].(map[string]any)
			if !ok {
				t.Fatalf("mcpServers = %v, want an object", config["mcpServers"])
			}
			other, ok := servers["other"].(map[string]any)
			if !ok {
				t.Fatalf("mcpServers.other = %v, want an object", servers["other"])
			}
			if other["command"] != "other-server" {
				t.Errorf("other command = %v, want other-server", other["command"])
			}
			if want := []any{"--other"}; !reflect.DeepEqual(other["args"], want) {
				t.Errorf("other args = %v, want %v", other["args"], want)
			}

			entry, ok := servers["pandora"].(map[string]any)
			if !ok {
				t.Fatalf("mcpServers.pandora = %v, want an object", servers["pandora"])
			}
			if entry["command"] != "/opt/pandora/bin/pandora" {
				t.Errorf("command = %v, want /opt/pandora/bin/pandora", entry["command"])
			}
			if want := []any{"serve"}; !reflect.DeepEqual(entry["args"], want) {
				t.Errorf("args = %v, want %v", entry["args"], want)
			}
			if tt.client == ClaudeCode {
				if entry["type"] != "stdio" {
					t.Errorf("type = %v, want stdio", entry["type"])
				}
				envValues, ok := entry["env"].(map[string]any)
				if !ok || len(envValues) != 0 {
					t.Errorf("env = %v, want an empty object", entry["env"])
				}
			} else if _, ok := entry["type"]; ok {
				t.Errorf("type = %v, want no type field", entry["type"])
			}
		})
	}
}

func TestAnExistingConfigIsBackedUpBeforeTheFirstChange(t *testing.T) {
	env := testEnv(t)
	path := mustConfigPath(t, env, Cursor)
	original := []byte(`{"mcpServers":{"other":{"command":"other-server"}}}` + "\n")
	writeConfig(t, path, string(original), 0o600)

	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}
	res := mustInstall(t, env, Cursor, e)
	wantBackup := path + ".pandora.bak"
	if res.BackupPath != wantBackup {
		t.Fatalf("BackupPath = %q, want %q", res.BackupPath, wantBackup)
	}
	if backup := readConfig(t, wantBackup); !bytes.Equal(backup, original) {
		t.Errorf("backup = %q, want %q", backup, original)
	}
	info, err := os.Stat(wantBackup)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", wantBackup, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %o, want 600", info.Mode().Perm())
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %o, want 600", info.Mode().Perm())
	}

	second := mustInstall(t, env, Cursor, e)
	if second.Changed {
		t.Error("second Install() Changed = true, want false")
	}
	if second.BackupPath != "" {
		t.Errorf("second BackupPath = %q, want empty", second.BackupPath)
	}
}

func TestClaudeCodeIsConfiguredThroughTheClaudeCLIWhenItIsInstalled(t *testing.T) {
	env := testEnv(t)
	configPath := mustConfigPath(t, env, ClaudeCode)
	var calls [][]string
	env.LookPath = func(file string) (string, error) {
		if file == "claude" {
			return "/usr/local/bin/claude", nil
		}
		return "", exec.ErrNotFound
	}
	env.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if len(args) >= 2 && args[0] == "mcp" && args[1] == "add" {
			content := `{"mcpServers":{"pandora":{"type":"stdio","command":"/opt/pandora/bin/pandora","args":["serve"],"env":{}}}}` + "\n"
			if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
				return nil, err
			}
		}
		return []byte("ok"), nil
	}

	e := Entry{Name: "pandora", Command: "/opt/pandora/bin/pandora", Args: []string{"serve"}}
	res := mustInstall(t, env, ClaudeCode, e)
	if res.Method != "cli" {
		t.Errorf("Method = %q, want cli", res.Method)
	}
	if !res.Changed {
		t.Error("Install() Changed = false, want true")
	}
	wantCalls := [][]string{
		{"claude", "mcp", "remove", "--scope", "user", "pandora"},
		{"claude", "mcp", "add", "--scope", "user", "pandora", "--", "/opt/pandora/bin/pandora", "serve"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("calls = %v, want %v", calls, wantCalls)
	}

	installed, err := IsInstalled(context.Background(), env, ClaudeCode, "pandora")
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !installed {
		t.Error("IsInstalled() = false, want true")
	}

	second := mustInstall(t, env, ClaudeCode, e)
	if second.Changed {
		t.Error("second Install() Changed = true, want false")
	}
	if len(calls) != len(wantCalls) {
		t.Errorf("calls after second install = %v, want no new calls", calls[len(wantCalls):])
	}
}
