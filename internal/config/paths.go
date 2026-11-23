package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigDir string
	CacheDir  string
	StateDir  string
}

func DefaultPaths(getenv func(string) string) (Paths, error) {
	if home := getenv("PANDORA_HOME"); home != "" {
		return Paths{
			ConfigDir: filepath.Join(home, "config"),
			CacheDir:  filepath.Join(home, "cache"),
			StateDir:  filepath.Join(home, "state"),
		}, nil
	}

	home := getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("finding home directory: %w", err)
		}
	}

	return Paths{
		ConfigDir: filepath.Join(xdgDir(getenv, "XDG_CONFIG_HOME", filepath.Join(home, ".config")), "pandora"),
		CacheDir:  filepath.Join(xdgDir(getenv, "XDG_CACHE_HOME", filepath.Join(home, ".cache")), "pandora"),
		StateDir:  filepath.Join(xdgDir(getenv, "XDG_STATE_HOME", filepath.Join(home, ".local", "state")), "pandora"),
	}, nil
}

func xdgDir(getenv func(string) string, name, fallback string) string {
	if dir := getenv(name); dir != "" {
		return dir
	}
	return fallback
}

func (p Paths) ConfigFile() string {
	return filepath.Join(p.ConfigDir, "config.toml")
}

func (p Paths) LockFile() string {
	return filepath.Join(p.ConfigDir, "config.lock")
}

func (p Paths) SecretsFile() string {
	return filepath.Join(p.ConfigDir, "secrets.json")
}

func (p Paths) AccountLock(service, account string) string {
	return filepath.Join(p.StateDir, "locks", service+"__"+account+".lock")
}

func (p Paths) ToolCacheFile(service string) string {
	return filepath.Join(p.CacheDir, "tools", service+".json")
}

func (p Paths) UpstreamLog(service, account string) string {
	return filepath.Join(p.StateDir, "logs", service+"__"+account+".log")
}

func (p Paths) DaemonSocket() string {
	path := filepath.Join(p.StateDir, "pandora.sock")
	if len(path) <= 100 {
		return path
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("pandora-%d", os.Getuid()), "pandora.sock")
}

func (p Paths) DaemonLog() string {
	return filepath.Join(p.StateDir, "logs", "daemon.log")
}
