package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

type Autostart struct {
	Exe        string
	Home       string
	Env        map[string]string
	LogPath    string
	UID        int
	ConfigHome string
	Runner     tunnel.Runner
}

func AutostartEnv(getenv func(string) string, environ []string) map[string]string {
	env := make(map[string]string)
	for _, name := range []string{"PATH", "HOME"} {
		if value := getenv(name); value != "" {
			env[name] = value
		}
	}
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || !strings.HasPrefix(name, "PANDORA_") {
			continue
		}
		env[name] = value
	}
	return env
}

func (a Autostart) runner() tunnel.Runner {
	if a.Runner != nil {
		return a.Runner
	}
	return tunnel.ExecRunner{}
}

func writeAutostartFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	return nil
}

func removeAutostartFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}

func autostartFileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("checking %s: %w", path, err)
	}
	return false, nil
}

func autostartCommandError(action string, stderr []byte, err error) error {
	message := strings.TrimSpace(string(stderr))
	message, _, _ = strings.Cut(message, "\n")
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}
