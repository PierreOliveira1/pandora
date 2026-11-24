//go:build linux

package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const systemdUnit = "pandora.service"

func (a Autostart) Enable(ctx context.Context) error {
	if err := writeAutostartFile(a.servicePath(), a.serviceContent()); err != nil {
		return err
	}
	if _, stderr, err := a.runner().Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return autostartCommandError("reloading the user systemd daemon", stderr, err)
	}
	if _, stderr, err := a.runner().Run(ctx, "systemctl", "--user", "enable", "--now", systemdUnit); err != nil {
		return autostartCommandError("enabling the Pandora service", stderr, err)
	}
	return nil
}

func (a Autostart) Disable(ctx context.Context) error {
	if _, stderr, err := a.runner().Run(ctx, "systemctl", "--user", "disable", "--now", systemdUnit); err != nil && !missingSystemdUnit(stderr, err) {
		return autostartCommandError("disabling the Pandora service", stderr, err)
	}
	if err := removeAutostartFile(a.servicePath()); err != nil {
		return err
	}
	if _, stderr, err := a.runner().Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return autostartCommandError("reloading the user systemd daemon", stderr, err)
	}
	return nil
}

func (a Autostart) Enabled(ctx context.Context) (bool, error) {
	return autostartFileExists(a.servicePath())
}

func (a Autostart) servicePath() string {
	configHome := a.ConfigHome
	if configHome == "" {
		configHome = filepath.Join(a.Home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user", systemdUnit)
}

func (a Autostart) serviceContent() []byte {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Pandora daemon\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("ExecStart=")
	b.WriteString(systemdQuote(a.Exe))
	b.WriteString(" daemon\n")
	keys := make([]string, 0, len(a.Env))
	for key := range a.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "Environment=%q\n", systemdEscapeSpecifiers(key+"="+a.Env[key]))
	}
	b.WriteString("Restart=on-failure\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return []byte(b.String())
}

func systemdEscapeSpecifiers(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

func systemdQuote(value string) string {
	value = systemdEscapeSpecifiers(value)
	if !strings.ContainsAny(value, " \t") {
		return value
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func missingSystemdUnit(stderr []byte, err error) bool {
	message := strings.ToLower(string(stderr) + " " + err.Error())
	for _, fragment := range []string{"not loaded", "not found", "does not exist", "no such file"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
