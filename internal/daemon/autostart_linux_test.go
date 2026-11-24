//go:build linux

package daemon_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/daemon"
)

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

func TestEnableWritesTheUserServiceAndStartsIt(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "Pandora App", "pandora")
	runner := &fakeRunner{}
	autostart := daemon.Autostart{
		Exe:    exe,
		Home:   home,
		Env:    map[string]string{"PATH": "/usr/bin:/bin", "HOME": home, "PANDORA_HOME": "/tmp/pandora"},
		Runner: runner,
	}

	if err := autostart.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	path := filepath.Join(home, ".config", "systemd", "user", "pandora.service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading user service: %v", err)
	}
	for _, want := range []string{
		"[Unit]",
		"Description=Pandora daemon",
		"ExecStart=\"" + exe + "\" daemon",
		`Environment="HOME=` + home + `"`,
		`Environment="PANDORA_HOME=/tmp/pandora"`,
		`Environment="PATH=/usr/bin:/bin"`,
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("user service does not contain %q:\n%s", want, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("checking user service: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("user service mode = %o, want 644", got)
	}
	wantCalls := []runnerCall{
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
		{name: "systemctl", args: []string{"--user", "enable", "--now", "pandora.service"}},
	}
	if !slices.EqualFunc(runner.calls, wantCalls, runnerCallEqual) {
		t.Fatalf("runner calls = %v, want %v", runner.calls, wantCalls)
	}
}

func TestDisableStopsTheUserServiceAndRemovesIt(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "systemd", "user", "pandora.service")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating systemd directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("service"), 0o644); err != nil {
		t.Fatalf("writing user service: %v", err)
	}
	runner := &fakeRunner{results: []runnerResult{
		{stderr: []byte("Unit pandora.service not loaded."), err: errors.New("exit status 1")},
	}}
	autostart := daemon.Autostart{Home: home, Runner: runner}

	if err := autostart.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("user service still exists: %v", err)
	}
	want := []runnerCall{
		{name: "systemctl", args: []string{"--user", "disable", "--now", "pandora.service"}},
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
	}
	if !slices.EqualFunc(runner.calls, want, runnerCallEqual) {
		t.Fatalf("runner calls = %v, want %v", runner.calls, want)
	}
}

func TestEnabledReflectsTheUserServiceFile(t *testing.T) {
	home := t.TempDir()
	autostart := daemon.Autostart{Home: home}

	enabled, err := autostart.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if enabled {
		t.Fatal("Enabled() = true without a user service")
	}
	path := filepath.Join(home, ".config", "systemd", "user", "pandora.service")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating systemd directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("service"), 0o644); err != nil {
		t.Fatalf("writing user service: %v", err)
	}
	enabled, err = autostart.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("Enabled() = false with a user service")
	}
}

func runnerCallEqual(a, b runnerCall) bool {
	return a.name == b.name && slices.Equal(a.args, b.args)
}
