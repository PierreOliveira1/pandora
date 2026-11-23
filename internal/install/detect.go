package install

import (
	"os"
	"path/filepath"
)

func DetectSkillClients(env Env) []Client {
	detected := make([]Client, 0, len(SkillClients()))
	for _, client := range SkillClients() {
		for _, path := range clientPaths(env, client) {
			if _, err := os.Stat(path); err == nil {
				detected = append(detected, client)
				break
			}
		}
	}
	return detected
}

func clientPaths(env Env, c Client) []string {
	switch c {
	case ClaudeCode:
		return []string{filepath.Join(env.Home, ".claude")}
	case OpenCode:
		return []string{filepath.Join(env.Home, ".config", "opencode")}
	case Codex:
		return []string{
			filepath.Join(env.Home, ".codex"),
			filepath.Join(env.Home, ".agents"),
		}
	default:
		return nil
	}
}
