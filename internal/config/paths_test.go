package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func longHome(t *testing.T, name string) string {
	return filepath.Join(t.TempDir(), strings.Repeat("deep-folder-", 8), name)
}

func TestTwoHomesWithLongPathsReachDifferentDaemons(t *testing.T) {
	first, err := config.DefaultPaths(func(key string) string {
		if key == "PANDORA_HOME" {
			return longHome(t, "first")
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.DefaultPaths(func(key string) string {
		if key == "PANDORA_HOME" {
			return longHome(t, "second")
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.DaemonSocket() == second.DaemonSocket() {
		t.Fatalf("both homes use the daemon socket %s", first.DaemonSocket())
	}
	if len(first.DaemonSocket()) > 100 {
		t.Fatalf("daemon socket path has %d characters, over the unix socket limit", len(first.DaemonSocket()))
	}
}
