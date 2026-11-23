package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

var ErrNoPermissionConfig = errors.New("client has no permission config")

const claudeCodePermission = "Bash(pandora:*)"

func AllowCommand(ctx context.Context, env Env, c Client) (Result, error) {
	switch c {
	case ClaudeCode:
		return allowClaudeCode(ctx, env)
	case OpenCode:
		return allowOpenCode(ctx, env)
	default:
		return Result{Client: c}, ErrNoPermissionConfig
	}
}

func DisallowCommand(ctx context.Context, env Env, c Client) (Result, error) {
	switch c {
	case ClaudeCode:
		return disallowClaudeCode(ctx, env)
	case OpenCode:
		return disallowOpenCode(ctx, env)
	default:
		return Result{Client: c}, ErrNoPermissionConfig
	}
}

func allowClaudeCode(ctx context.Context, env Env) (Result, error) {
	path := filepath.Join(env.Home, ".claude", "settings.json")
	res := Result{Client: ClaudeCode, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, ClaudeCode, path, permissionSnippet(ClaudeCode))
	if err != nil {
		return res, err
	}
	permissions, err := objectAt(config, "permissions")
	if err != nil {
		return res, err
	}
	allow, err := permissionList(permissions, "allow")
	if err != nil {
		return res, err
	}
	if containsPermission(allow, claudeCodePermission) {
		return res, nil
	}
	permissions["allow"] = append(allow, claudeCodePermission)
	config["permissions"] = permissions
	return savePermissionConfig(ctx, res, path, config, data, mode, exists)
}

func disallowClaudeCode(ctx context.Context, env Env) (Result, error) {
	path := filepath.Join(env.Home, ".claude", "settings.json")
	res := Result{Client: ClaudeCode, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, ClaudeCode, path, permissionSnippet(ClaudeCode))
	if err != nil {
		return res, err
	}
	if !exists {
		return res, nil
	}
	permissions, ok := config["permissions"].(map[string]any)
	if !ok {
		return res, nil
	}
	allow, ok := permissions["allow"].([]any)
	if !ok {
		return res, nil
	}
	filtered, changed := removePermission(allow, claudeCodePermission)
	if !changed {
		return res, nil
	}
	permissions["allow"] = filtered
	config["permissions"] = permissions
	return savePermissionConfig(ctx, res, path, config, data, mode, exists)
}

func allowOpenCode(ctx context.Context, env Env) (Result, error) {
	path := filepath.Join(env.Home, ".config", "opencode", "opencode.json")
	res := Result{Client: OpenCode, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, OpenCode, path, permissionSnippet(OpenCode))
	if err != nil {
		return res, err
	}
	permission, err := objectAt(config, "permission")
	if err != nil {
		return res, err
	}
	bash := map[string]any{}
	switch value := permission["bash"].(type) {
	case nil:
	case string:
		bash["*"] = value
	case map[string]any:
		bash = value
	default:
		return res, fmt.Errorf("permission.bash in %s must be a string or an object", path)
	}
	if value, ok := bash["pandora *"].(string); ok && value == "allow" {
		return res, nil
	}
	bash["pandora *"] = "allow"
	permission["bash"] = bash
	config["permission"] = permission
	return savePermissionConfig(ctx, res, path, config, data, mode, exists)
}

func disallowOpenCode(ctx context.Context, env Env) (Result, error) {
	path := filepath.Join(env.Home, ".config", "opencode", "opencode.json")
	res := Result{Client: OpenCode, Method: "file", ConfigPath: path}
	config, data, mode, exists, err := loadJSONConfig(ctx, OpenCode, path, permissionSnippet(OpenCode))
	if err != nil {
		return res, err
	}
	if !exists {
		return res, nil
	}
	permission, ok := config["permission"].(map[string]any)
	if !ok {
		return res, nil
	}
	bash, ok := permission["bash"].(map[string]any)
	if !ok {
		return res, nil
	}
	if _, ok := bash["pandora *"]; !ok {
		return res, nil
	}
	delete(bash, "pandora *")
	permission["bash"] = bash
	config["permission"] = permission
	return savePermissionConfig(ctx, res, path, config, data, mode, exists)
}

func savePermissionConfig(ctx context.Context, res Result, path string, config map[string]any, original []byte, mode fs.FileMode, exists bool) (Result, error) {
	if err := ctx.Err(); err != nil {
		return res, err
	}
	out, err := marshalConfig(config)
	if err != nil {
		return res, err
	}
	if exists {
		backup, err := backupFile(path, original, mode)
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

func permissionList(object map[string]any, key string) ([]any, error) {
	value, ok := object[key]
	if !ok || value == nil {
		return []any{}, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("permissions.%s must be a JSON array", key)
	}
	return list, nil
}

func containsPermission(list []any, permission string) bool {
	for _, item := range list {
		if value, ok := item.(string); ok && value == permission {
			return true
		}
	}
	return false
}

func removePermission(list []any, permission string) ([]any, bool) {
	filtered := make([]any, 0, len(list))
	changed := false
	for _, item := range list {
		if value, ok := item.(string); ok && value == permission {
			changed = true
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, changed
}

func permissionSnippet(c Client) string {
	switch c {
	case ClaudeCode:
		return "{\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(pandora:*)\"\n    ]\n  }\n}"
	case OpenCode:
		return "{\n  \"permission\": {\n    \"bash\": {\n      \"pandora *\": \"allow\"\n    }\n  }\n}"
	default:
		return ""
	}
}
