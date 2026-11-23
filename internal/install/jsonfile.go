package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

type unsupportedFormatError struct {
	path    string
	snippet string
}

func (e *unsupportedFormatError) Error() string {
	return fmt.Sprintf("%s could not be parsed as JSON; add this manually:\n%s", e.path, e.snippet)
}

func (e *unsupportedFormatError) Unwrap() error {
	return ErrUnsupportedFormat
}

func installJSON(ctx context.Context, c Client, path string, e Entry) (Result, error) {
	res := Result{Client: c, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, c, path, Snippet(c, e))
	if err != nil {
		return res, err
	}
	servers, err := objectAt(config, topKey(c))
	if err != nil {
		return res, err
	}
	desired := jsonEntry(c, e)
	if current, ok := servers[e.Name]; ok && deepEqualJSON(current, desired) {
		return res, nil
	}
	if !exists && c == OpenCode {
		if _, ok := config["$schema"]; !ok {
			config["$schema"] = "https://opencode.ai/config.json"
		}
	}
	servers[e.Name] = desired
	config[topKey(c)] = servers
	out, err := marshalConfig(config)
	if err != nil {
		return res, err
	}
	if exists {
		backup, err := backupFile(path, data, mode)
		if err != nil {
			return res, err
		}
		res.BackupPath = backup
	}
	if err := writeFileAtomic(path, out, modeFor(mode, exists)); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

func uninstallJSON(ctx context.Context, c Client, path, name string) (Result, error) {
	res := Result{Client: c, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, c, path, Snippet(c, Entry{Name: name}))
	if err != nil {
		return res, err
	}
	if !exists {
		return res, nil
	}
	servers, err := objectAt(config, topKey(c))
	if err != nil {
		return res, err
	}
	if _, ok := servers[name]; !ok {
		return res, nil
	}
	delete(servers, name)
	config[topKey(c)] = servers
	out, err := marshalConfig(config)
	if err != nil {
		return res, err
	}
	backup, err := backupFile(path, data, mode)
	if err != nil {
		return res, err
	}
	res.BackupPath = backup
	if err := writeFileAtomic(path, out, modeFor(mode, exists)); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

func isInstalledJSON(ctx context.Context, c Client, path, name string) (bool, error) {
	config, _, _, exists, err := loadJSONConfig(ctx, c, path, Snippet(c, Entry{Name: name}))
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	value, ok := config[topKey(c)]
	if !ok {
		return false, nil
	}
	servers, ok := value.(map[string]any)
	if !ok {
		return false, fmt.Errorf("%s must be a JSON object", topKey(c))
	}
	_, ok = servers[name]
	return ok, nil
}

func readJSONEntry(ctx context.Context, c Client, path, name string) (map[string]any, error) {
	config, _, _, exists, err := loadJSONConfig(ctx, c, path, Snippet(c, Entry{Name: name}))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	value, ok := config[topKey(c)]
	if !ok {
		return nil, nil
	}
	servers, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", topKey(c))
	}
	entry, ok := servers[name].(map[string]any)
	if !ok {
		return nil, nil
	}
	return entry, nil
}

func loadJSONConfig(ctx context.Context, c Client, path, snippet string) (map[string]any, []byte, fs.FileMode, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, 0, false, err
	}
	data, mode, exists, err := readConfigFile(path)
	if err != nil {
		return nil, nil, 0, false, err
	}
	if !exists {
		if c == OpenCode {
			jsoncPath := path + "c"
			if _, err := os.Stat(jsoncPath); err == nil {
				return nil, nil, 0, false, &unsupportedFormatError{path: path, snippet: snippet}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, 0, false, fmt.Errorf("checking %s: %w", jsoncPath, err)
			}
		}
		return map[string]any{}, nil, 0, false, nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, nil, 0, false, &unsupportedFormatError{path: path, snippet: snippet}
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, data, mode, true, nil
}

func objectAt(config map[string]any, key string) (map[string]any, error) {
	value, ok := config[key]
	if !ok || value == nil {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	return object, nil
}

func marshalConfig(config map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	return append(data, '\n'), nil
}

func deepEqualJSON(a, b any) bool {
	return reflect.DeepEqual(normalizeJSON(a), normalizeJSON(b))
}

func normalizeJSON(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return value
	}
	return normalized
}

func readConfigFile(path string) ([]byte, fs.FileMode, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading %s: %w", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, info.Mode().Perm(), true, nil
}

func modeFor(mode fs.FileMode, exists bool) fs.FileMode {
	if exists {
		return mode
	}
	return 0o644
}

func backupFile(path string, data []byte, mode fs.FileMode) (string, error) {
	backup := path + ".pandora.bak"
	if err := os.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("removing old backup %s: %w", backup, err)
	}
	if err := os.WriteFile(backup, data, mode); err != nil {
		return "", fmt.Errorf("backing up %s: %w", path, err)
	}
	if err := os.Chmod(backup, mode); err != nil {
		return "", fmt.Errorf("setting mode on %s: %w", backup, err)
	}
	return backup, nil
}

func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".pandora-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary file in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("setting mode on %s: %w", tempPath, err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("writing %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	removeTemp = false
	return nil
}
