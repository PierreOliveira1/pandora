//go:build darwin

package daemon

import (
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	launchdLabel   = "io.github.pierreoliveira1.pandora"
	launchAgentDir = "Library/LaunchAgents"
)

func (a Autostart) Enable(ctx context.Context) error {
	path := a.plistPath()
	if err := writeAutostartFile(path, a.plistContent()); err != nil {
		return err
	}
	_, _, _ = a.runner().Run(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", a.UID, launchdLabel))
	if _, stderr, err := a.runner().Run(ctx, "launchctl", "bootstrap", fmt.Sprintf("gui/%d", a.UID), path); err != nil {
		return autostartCommandError("loading the login item", stderr, err)
	}
	return nil
}

func (a Autostart) Disable(ctx context.Context) error {
	_, _, _ = a.runner().Run(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", a.UID, launchdLabel))
	return removeAutostartFile(a.plistPath())
}

func (a Autostart) Enabled(ctx context.Context) (bool, error) {
	return autostartFileExists(a.plistPath())
}

func (a Autostart) plistPath() string {
	return filepath.Join(a.Home, launchAgentDir, launchdLabel+".plist")
}

func (a Autostart) plistContent() []byte {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n")
	b.WriteString("<dict>\n")
	b.WriteString("\t<key>Label</key>\n")
	writePlistText(&b, launchdLabel, "\t")
	b.WriteString("\n\t<key>ProgramArguments</key>\n\t<array>\n")
	writePlistText(&b, a.Exe, "\t\t")
	b.WriteString("\n\t\t<string>daemon</string>\n\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n")
	b.WriteString("\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
	keys := make([]string, 0, len(a.Env))
	for key := range a.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString("\t\t<key>")
		_ = xml.EscapeText(&b, []byte(key))
		b.WriteString("</key>\n")
		writePlistText(&b, a.Env[key], "\t\t")
		b.WriteByte('\n')
	}
	b.WriteString("\t</dict>\n")
	b.WriteString("\t<key>StandardOutPath</key>\n")
	writePlistText(&b, a.LogPath, "\t")
	b.WriteString("\n\t<key>StandardErrorPath</key>\n")
	writePlistText(&b, a.LogPath, "\t")
	b.WriteString("\n</dict>\n</plist>\n")
	return []byte(b.String())
}

func writePlistText(b *strings.Builder, value, indent string) {
	b.WriteString(indent)
	b.WriteString("<string>")
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</string>")
}
