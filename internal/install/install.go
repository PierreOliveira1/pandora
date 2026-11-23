package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type Client string

const (
	ClaudeCode    Client = "claude-code"
	Codex         Client = "codex"
	OpenCode      Client = "opencode"
	Cursor        Client = "cursor"
	ClaudeDesktop Client = "claude-desktop"
)

var ErrUnsupportedFormat = errors.New("unsupported config format")

func Clients() []Client {
	return []Client{ClaudeCode, Codex, OpenCode, Cursor, ClaudeDesktop}
}

func ParseClient(s string) (Client, error) {
	switch s {
	case string(ClaudeCode), "claude":
		return ClaudeCode, nil
	case string(Codex):
		return Codex, nil
	case string(OpenCode):
		return OpenCode, nil
	case string(Cursor):
		return Cursor, nil
	case string(ClaudeDesktop), "desktop":
		return ClaudeDesktop, nil
	default:
		return "", fmt.Errorf("unknown client %q: valid clients are %s, %s, %s, %s and %s", s, ClaudeCode, Codex, OpenCode, Cursor, ClaudeDesktop)
	}
}

type Entry struct {
	Name    string
	Command string
	Args    []string
}

type Env struct {
	Home       string
	GOOS       string
	Executable string
	LookPath   func(file string) (string, error)
	Run        func(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Result struct {
	Client     Client
	Method     string
	ConfigPath string
	Changed    bool
	BackupPath string
}

func ConfigPath(env Env, c Client) (string, error) {
	switch c {
	case ClaudeCode:
		return filepath.Join(env.Home, ".claude.json"), nil
	case Codex:
		return filepath.Join(env.Home, ".codex", "config.toml"), nil
	case OpenCode:
		return filepath.Join(env.Home, ".config", "opencode", "opencode.json"), nil
	case Cursor:
		return filepath.Join(env.Home, ".cursor", "mcp.json"), nil
	case ClaudeDesktop:
		switch env.GOOS {
		case "darwin":
			return filepath.Join(env.Home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		case "linux":
			return filepath.Join(env.Home, ".config", "Claude", "claude_desktop_config.json"), nil
		case "windows":
			return filepath.Join(env.Home, "AppData", "Roaming", "Claude", "claude_desktop_config.json"), nil
		default:
			return "", fmt.Errorf("claude-desktop is not supported on %s", env.GOOS)
		}
	default:
		return "", fmt.Errorf("unknown client %q", c)
	}
}

func Install(ctx context.Context, env Env, c Client, e Entry) (Result, error) {
	path, err := ConfigPath(env, c)
	if err != nil {
		return Result{Client: c}, err
	}
	if c == ClaudeCode && claudeAvailable(env) {
		return installClaudeCLI(ctx, env, path, e)
	}
	if c == Codex {
		return installCodex(ctx, path, e)
	}
	return installJSON(ctx, c, path, e)
}

func Uninstall(ctx context.Context, env Env, c Client, name string) (Result, error) {
	path, err := ConfigPath(env, c)
	if err != nil {
		return Result{Client: c}, err
	}
	if c == ClaudeCode && claudeAvailable(env) {
		return uninstallClaudeCLI(ctx, env, path, name)
	}
	if c == Codex {
		return uninstallCodex(ctx, path, name)
	}
	return uninstallJSON(ctx, c, path, name)
}

func IsInstalled(ctx context.Context, env Env, c Client, name string) (bool, error) {
	path, err := ConfigPath(env, c)
	if err != nil {
		return false, err
	}
	if c == Codex {
		return isInstalledCodex(ctx, path, name)
	}
	return isInstalledJSON(ctx, c, path, name)
}

func Snippet(c Client, e Entry) string {
	switch c {
	case ClaudeCode:
		args := []string{"claude", "mcp", "add", "--scope", "user", e.Name, "--", e.Command}
		args = append(args, e.Args...)
		return strings.Join(args, " ")
	case Codex:
		return strings.Join(codexBlock(e), "\n")
	case OpenCode, Cursor, ClaudeDesktop:
		config := map[string]any{topKey(c): map[string]any{e.Name: jsonEntry(c, e)}}
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return ""
		}
		return string(data)
	default:
		return ""
	}
}

func claudeAvailable(env Env) bool {
	if env.LookPath == nil {
		return false
	}
	_, err := env.LookPath("claude")
	return err == nil
}

func installClaudeCLI(ctx context.Context, env Env, path string, e Entry) (Result, error) {
	res := Result{Client: ClaudeCode, Method: "cli", ConfigPath: path}
	existing, err := readJSONEntry(ctx, ClaudeCode, path, e.Name)
	if err != nil {
		return res, err
	}
	if entryMatches(existing, e) {
		return res, nil
	}
	if env.Run == nil {
		return res, errors.New("claude command cannot be run: Run is not configured")
	}
	_, _ = env.Run(ctx, "claude", "mcp", "remove", "--scope", "user", e.Name)
	args := []string{"mcp", "add", "--scope", "user", e.Name, "--", e.Command}
	args = append(args, e.Args...)
	output, err := env.Run(ctx, "claude", args...)
	if err != nil {
		return res, fmt.Errorf("running claude mcp add for %s: %w: %s", e.Name, err, strings.TrimSpace(string(output)))
	}
	res.Changed = true
	return res, nil
}

func uninstallClaudeCLI(ctx context.Context, env Env, path, name string) (Result, error) {
	res := Result{Client: ClaudeCode, Method: "cli", ConfigPath: path}
	installed, err := IsInstalled(ctx, env, ClaudeCode, name)
	if err != nil {
		return res, err
	}
	if !installed {
		return res, nil
	}
	if env.Run == nil {
		return res, errors.New("claude command cannot be run: Run is not configured")
	}
	if _, err := env.Run(ctx, "claude", "mcp", "remove", "--scope", "user", name); err != nil {
		return res, fmt.Errorf("running claude mcp remove for %s: %w", name, err)
	}
	res.Changed = true
	return res, nil
}

func entryMatches(existing map[string]any, e Entry) bool {
	if existing == nil {
		return false
	}
	command, ok := existing["command"].(string)
	if !ok || command != e.Command {
		return false
	}
	return deepEqualJSON(existing["args"], entryArgs(e.Args))
}

func jsonEntry(c Client, e Entry) map[string]any {
	switch c {
	case ClaudeCode:
		return map[string]any{
			"type":    "stdio",
			"command": e.Command,
			"args":    entryArgs(e.Args),
			"env":     map[string]any{},
		}
	case OpenCode:
		command := make([]any, 0, len(e.Args)+1)
		command = append(command, e.Command)
		command = append(command, entryArgs(e.Args)...)
		return map[string]any{
			"type":    "local",
			"command": command,
			"enabled": true,
		}
	default:
		return map[string]any{
			"command": e.Command,
			"args":    entryArgs(e.Args),
		}
	}
}

func entryArgs(args []string) []any {
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = arg
	}
	return out
}

func topKey(c Client) string {
	switch c {
	case OpenCode:
		return "mcp"
	case ClaudeCode, Cursor, ClaudeDesktop:
		return "mcpServers"
	default:
		return ""
	}
}
