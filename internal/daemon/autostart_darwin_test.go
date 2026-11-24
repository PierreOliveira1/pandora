//go:build darwin

package daemon_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/daemon"
)

const launchAgentPath = "Library/LaunchAgents/io.github.pierreoliveira1.pandora.plist"

type runnerCall struct {
	name string
	args []string
}

type runnerResult struct {
	stdout []byte
	stderr []byte
	err    error
}

type fakeRunner struct {
	calls   []runnerCall
	results []runnerResult
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if len(f.results) == 0 {
		return nil, nil, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result.stdout, result.stderr, result.err
}

func TestEnableWritesTheLaunchAgentAndLoadsIt(t *testing.T) {
	home := t.TempDir()
	logPath := filepath.Join(home, "state", "daemon.log")
	runner := &fakeRunner{}
	autostart := daemon.Autostart{
		Exe:     "/opt/pandora/bin/pandora",
		Home:    home,
		Env:     map[string]string{"PATH": "/usr/local/bin:/usr/bin", "HOME": home},
		LogPath: logPath,
		UID:     501,
		Runner:  runner,
	}

	if err := autostart.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	path := filepath.Join(home, launchAgentPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading LaunchAgent: %v", err)
	}
	for _, want := range []string{
		"<string>/opt/pandora/bin/pandora</string>",
		"<string>daemon</string>",
		"<key>PATH</key>",
		"<string>/usr/local/bin:/usr/bin</string>",
		"<string>" + logPath + "</string>",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("LaunchAgent does not contain %q:\n%s", want, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("checking LaunchAgent: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("LaunchAgent mode = %o, want 644", got)
	}
	wantCalls := []runnerCall{
		{name: "launchctl", args: []string{"bootout", "gui/501/io.github.pierreoliveira1.pandora"}},
		{name: "launchctl", args: []string{"bootstrap", "gui/501", path}},
	}
	if !slices.EqualFunc(runner.calls, wantCalls, runnerCallEqual) {
		t.Fatalf("runner calls = %v, want %v", runner.calls, wantCalls)
	}
}

func TestDisableRemovesTheLaunchAgentAndUnloadsIt(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, launchAgentPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating LaunchAgents directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("plist"), 0o644); err != nil {
		t.Fatalf("writing LaunchAgent: %v", err)
	}
	runner := &fakeRunner{}
	autostart := daemon.Autostart{Home: home, UID: 501, Runner: runner}

	if err := autostart.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("LaunchAgent still exists: %v", err)
	}
	want := []runnerCall{{name: "launchctl", args: []string{"bootout", "gui/501/io.github.pierreoliveira1.pandora"}}}
	if !slices.EqualFunc(runner.calls, want, runnerCallEqual) {
		t.Fatalf("runner calls = %v, want %v", runner.calls, want)
	}
}

func TestEnabledReflectsTheLaunchAgentFile(t *testing.T) {
	home := t.TempDir()
	autostart := daemon.Autostart{Home: home}

	enabled, err := autostart.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if enabled {
		t.Fatal("Enabled() = true without a LaunchAgent")
	}
	path := filepath.Join(home, launchAgentPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating LaunchAgents directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("plist"), 0o644); err != nil {
		t.Fatalf("writing LaunchAgent: %v", err)
	}
	enabled, err = autostart.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("Enabled() = false with a LaunchAgent")
	}
}

func TestLaunchAgentEscapesAnAmpersandInTheExecutablePath(t *testing.T) {
	home := t.TempDir()
	autostart := daemon.Autostart{
		Exe:    filepath.Join(home, "a&b", "pandora"),
		Home:   home,
		Env:    map[string]string{},
		Runner: &fakeRunner{},
	}

	if err := autostart.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, launchAgentPath))
	if err != nil {
		t.Fatalf("reading LaunchAgent: %v", err)
	}
	if !strings.Contains(string(data), "a&amp;b") {
		t.Fatalf("LaunchAgent does not escape the ampersand:\n%s", data)
	}
	if strings.Contains(string(data), "a&b") {
		t.Fatalf("LaunchAgent contains an unescaped ampersand:\n%s", data)
	}
}

func TestAutostartEnvKeepsPathHomeAndPandoraVariables(t *testing.T) {
	getenv := func(name string) string {
		return map[string]string{"PATH": "/usr/bin:/bin", "HOME": "/home/me"}[name]
	}
	got := daemon.AutostartEnv(getenv, []string{
		"PATH=/ignored",
		"HOME=/ignored",
		"PANDORA_HOME=/tmp/pandora",
		"PANDORA_EMPTY=",
		"OTHER=value",
		"PANDORA_CLIENT=claude",
	})
	want := map[string]string{
		"PATH":           "/usr/bin:/bin",
		"HOME":           "/home/me",
		"PANDORA_HOME":   "/tmp/pandora",
		"PANDORA_CLIENT": "claude",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("AutostartEnv() = %v, want %v", got, want)
	}
}

func runnerCallEqual(a, b runnerCall) bool {
	return a.name == b.name && slices.Equal(a.args, b.args)
}
