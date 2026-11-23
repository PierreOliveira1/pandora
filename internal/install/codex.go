package install

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

func installCodex(ctx context.Context, path string, e Entry) (Result, error) {
	res := Result{Client: Codex, Method: "file", ConfigPath: path}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	data, mode, exists, err := readConfigFile(path)
	if err != nil {
		return res, err
	}
	lines := splitLines(string(data))
	block := codexBlock(e)
	start, end := findCodexBlock(lines, e.Name)
	if start < 0 {
		return saveConfigLines(res, path, data, mode, exists, appendCodexBlock(lines, block))
	}
	contentEnd := trimTrailingBlankLines(lines, start, end)
	if slices.Equal(lines[start:contentEnd], block) {
		return res, nil
	}
	return saveConfigLines(res, path, data, mode, exists, replaceLines(lines, start, contentEnd, block))
}

func uninstallCodex(ctx context.Context, path, name string) (Result, error) {
	res := Result{Client: Codex, Method: "file", ConfigPath: path}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	data, mode, exists, err := readConfigFile(path)
	if err != nil {
		return res, err
	}
	if !exists {
		return res, nil
	}
	lines := splitLines(string(data))
	start, end := findCodexBlock(lines, name)
	if start < 0 {
		return res, nil
	}
	contentEnd := trimTrailingBlankLines(lines, start, end)
	remaining := make([]string, 0, len(lines))
	remaining = append(remaining, lines[:start]...)
	rest := lines[contentEnd:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	remaining = append(remaining, rest...)
	for len(remaining) > 0 && strings.TrimSpace(remaining[len(remaining)-1]) == "" {
		remaining = remaining[:len(remaining)-1]
	}
	return saveConfigLines(res, path, data, mode, exists, remaining)
}

func isInstalledCodex(ctx context.Context, path, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	data, _, exists, err := readConfigFile(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	start, _ := findCodexBlock(splitLines(string(data)), name)
	return start >= 0, nil
}

func codexBlock(e Entry) []string {
	quoted := make([]string, len(e.Args))
	for i, arg := range e.Args {
		quoted[i] = tomlString(arg)
	}
	return []string{
		"[mcp_servers." + e.Name + "]",
		"command = " + tomlString(e.Command),
		"args = [" + strings.Join(quoted, ", ") + "]",
	}
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(fmt.Sprintf(`\u%04X`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func findCodexBlock(lines []string, name string) (int, int) {
	header := "[mcp_servers." + name + "]"
	prefix := "[mcp_servers." + name + "."
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, prefix) {
				end = j
				break
			}
		}
		return i, end
	}
	return -1, -1
}

func trimTrailingBlankLines(lines []string, start, end int) int {
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

func replaceLines(lines []string, start, end int, block []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(block))
	out = append(out, lines[:start]...)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return out
}

func appendCodexBlock(lines []string, block []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	out := make([]string, 0, len(lines)+len(block)+1)
	out = append(out, lines...)
	if len(out) > 0 {
		out = append(out, "")
	}
	out = append(out, block...)
	return out
}

func saveConfigLines(res Result, path string, original []byte, mode fs.FileMode, exists bool, lines []string) (Result, error) {
	if exists {
		backup, err := backupFile(path, original, mode)
		if err != nil {
			return res, err
		}
		res.BackupPath = backup
	}
	output := []byte{}
	if len(lines) > 0 {
		output = []byte(strings.Join(lines, "\n") + "\n")
	}
	if err := writeFileAtomic(path, output, modeFor(mode, exists)); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}
