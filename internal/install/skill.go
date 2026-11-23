package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	skillName          = "pandora"
	skillMarker        = ".pandora-managed"
	skillMarkerContent = "managed by pandora\n"
)

func SkillClients() []Client {
	return []Client{ClaudeCode, OpenCode, Codex}
}

func SkillDir(env Env, c Client) (string, error) {
	switch c {
	case ClaudeCode:
		return filepath.Join(env.Home, ".claude", "skills", skillName), nil
	case OpenCode:
		return filepath.Join(env.Home, ".config", "opencode", "skills", skillName), nil
	case Codex:
		return filepath.Join(env.Home, ".agents", "skills", skillName), nil
	default:
		return "", fmt.Errorf("%s does not support skills: use pandora install %s --mcp", c, c)
	}
}

func WriteSkill(ctx context.Context, env Env, c Client, files map[string][]byte) (Result, error) {
	dir, err := SkillDir(env, c)
	res := Result{Client: c, Method: "file", ConfigPath: dir}
	if err != nil {
		return res, err
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	desired, err := desiredSkillFiles(files)
	if err != nil {
		return res, err
	}
	info, err := os.Lstat(dir)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return res, fmt.Errorf("checking %s: %w", dir, err)
	}
	if exists {
		if !info.IsDir() {
			return res, skillConflictError(dir)
		}
		markerInfo, err := os.Lstat(filepath.Join(dir, skillMarker))
		if errors.Is(err, fs.ErrNotExist) {
			return res, skillConflictError(dir)
		}
		if err != nil {
			return res, fmt.Errorf("checking %s: %w", filepath.Join(dir, skillMarker), err)
		}
		if !markerInfo.Mode().IsRegular() {
			return res, skillConflictError(dir)
		}
		equal, err := skillTreeEqual(ctx, dir, desired)
		if err != nil {
			return res, err
		}
		if equal {
			return res, nil
		}
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return res, fmt.Errorf("creating %s: %w", parent, err)
	}
	temp, err := os.MkdirTemp(parent, filepath.Base(dir)+".tmp-*")
	if err != nil {
		return res, fmt.Errorf("creating temporary skill directory in %s: %w", parent, err)
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.RemoveAll(temp)
		}
	}()
	if err := writeSkillTree(ctx, temp, desired); err != nil {
		return res, err
	}
	if exists {
		old, err := os.MkdirTemp(parent, filepath.Base(dir)+".old-*")
		if err != nil {
			return res, fmt.Errorf("creating old skill directory in %s: %w", parent, err)
		}
		if err := os.Remove(old); err != nil {
			return res, fmt.Errorf("preparing %s: %w", old, err)
		}
		if err := os.Rename(dir, old); err != nil {
			return res, fmt.Errorf("moving old skill %s: %w", dir, err)
		}
		if err := os.Rename(temp, dir); err != nil {
			_ = os.Rename(old, dir)
			return res, fmt.Errorf("installing skill at %s: %w", dir, err)
		}
		removeTemp = false
		if err := os.RemoveAll(old); err != nil {
			return res, fmt.Errorf("removing old skill %s: %w", old, err)
		}
		res.Changed = true
		return res, nil
	}
	if err := os.Rename(temp, dir); err != nil {
		return res, fmt.Errorf("installing skill at %s: %w", dir, err)
	}
	removeTemp = false
	res.Changed = true
	return res, nil
}

func RemoveSkill(ctx context.Context, env Env, c Client) (Result, error) {
	dir, err := SkillDir(env, c)
	res := Result{Client: c, Method: "file", ConfigPath: dir}
	if err != nil {
		return res, err
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("checking %s: %w", dir, err)
	}
	if !info.IsDir() {
		return res, nil
	}
	markerInfo, err := os.Lstat(filepath.Join(dir, skillMarker))
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("checking %s: %w", filepath.Join(dir, skillMarker), err)
	}
	if !markerInfo.Mode().IsRegular() {
		return res, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return res, fmt.Errorf("removing skill at %s: %w", dir, err)
	}
	res.Changed = true
	return res, nil
}

func HasSkill(env Env, c Client) (bool, error) {
	dir, err := SkillDir(env, c)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", dir, err)
	}
	if !info.IsDir() {
		return false, nil
	}
	markerInfo, err := os.Lstat(filepath.Join(dir, skillMarker))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", filepath.Join(dir, skillMarker), err)
	}
	return markerInfo.Mode().IsRegular(), nil
}

func SkillUpToDate(ctx context.Context, env Env, c Client, files map[string][]byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	dir, err := SkillDir(env, c)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", dir, err)
	}
	if !info.IsDir() {
		return false, nil
	}
	markerInfo, err := os.Lstat(filepath.Join(dir, skillMarker))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", filepath.Join(dir, skillMarker), err)
	}
	if !markerInfo.Mode().IsRegular() {
		return false, nil
	}
	desired, err := desiredSkillFiles(files)
	if err != nil {
		return false, err
	}
	return skillTreeEqual(ctx, dir, desired)
}

func skillConflictError(dir string) error {
	return fmt.Errorf("a skill named pandora already exists at %s and was not created by Pandora; remove or rename it first", dir)
}

func desiredSkillFiles(files map[string][]byte) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files)+1)
	for name, content := range files {
		clean, err := cleanSkillPath(name)
		if err != nil {
			return nil, err
		}
		if clean == skillMarker {
			continue
		}
		if _, exists := out[clean]; exists {
			return nil, fmt.Errorf("duplicate skill file path %q", clean)
		}
		out[clean] = append([]byte(nil), content...)
	}
	out[skillMarker] = []byte(skillMarkerContent)
	return out, nil
}

func cleanSkillPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("skill file path is empty")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("skill file path %q must be relative", path)
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", fmt.Errorf("skill file path %q must not contain ..", path)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill file path %q must stay inside the skill directory", path)
	}
	if strings.ContainsRune(clean, 0) {
		return "", fmt.Errorf("skill file path %q is invalid", path)
	}
	return clean, nil
}

func writeSkillTree(ctx context.Context, root string, files map[string][]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, files[name], 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}

func skillTreeEqual(ctx context.Context, root string, desired map[string][]byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	directories := desiredDirectories(desired)
	found := make(map[string]bool, len(desired))
	equal := true
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !directories[relative] {
				equal = false
			}
			return nil
		}
		content, ok := desired[relative]
		if !ok || !entry.Type().IsRegular() {
			equal = false
			return nil
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, content) {
			equal = false
		}
		found[relative] = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("reading skill at %s: %w", root, err)
	}
	if !equal || len(found) != len(desired) {
		return false, nil
	}
	for name := range desired {
		if !found[name] {
			return false, nil
		}
	}
	return true, nil
}

func desiredDirectories(files map[string][]byte) map[string]bool {
	directories := map[string]bool{}
	for name := range files {
		for dir := filepath.Dir(name); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
			directories[dir] = true
		}
	}
	return directories
}
