package config_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func newTestStore(t *testing.T) *config.Store {
	t.Helper()
	root := t.TempDir()
	return config.NewStore(config.Paths{
		ConfigDir: filepath.Join(root, "config"),
		CacheDir:  filepath.Join(root, "cache"),
		StateDir:  filepath.Join(root, "state"),
	})
}

func TestConcurrentUpdatesNeverLoseAWrite(t *testing.T) {
	store := newTestStore(t)
	const updates = 20

	var wg sync.WaitGroup
	errs := make(chan error, updates)
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("service-%d", i)
			errs <- store.Update(func(cfg *config.Config) error {
				cfg.Services[name] = &config.Service{Kind: config.KindCommand}
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Services) != updates {
		t.Fatalf("got %d services, want %d", len(cfg.Services), updates)
	}
	for i := 0; i < updates; i++ {
		name := fmt.Sprintf("service-%d", i)
		if _, ok := cfg.Services[name]; !ok {
			t.Errorf("service %q was lost", name)
		}
	}
}
