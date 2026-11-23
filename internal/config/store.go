package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

const (
	lockTimeout = 10 * time.Second
	lockRetry   = 50 * time.Millisecond
)

type Store struct {
	paths Paths
}

func NewStore(p Paths) *Store {
	return &Store{paths: p}
}

func (s *Store) Paths() Paths {
	return s.paths
}

func (s *Store) Load() (*Config, error) {
	return s.load()
}

func (s *Store) Update(fn func(*Config) error) error {
	if err := os.MkdirAll(s.paths.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	lock := flock.New(s.paths.LockFile())
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, lockRetry)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("locking config file: timed out waiting for another process")
		}
		return fmt.Errorf("locking config file: %w", err)
	}
	if !locked {
		return fmt.Errorf("locking config file: timed out waiting for another process")
	}
	defer func() {
		_ = lock.Unlock()
	}()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	return s.write(cfg)
}

func (s *Store) load() (*Config, error) {
	data, err := os.ReadFile(s.paths.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Version: 1, Services: map[string]*Service{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", s.paths.ConfigFile(), err)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Services == nil {
		cfg.Services = map[string]*Service{}
	}
	return &cfg, nil
}

func (s *Store) write(cfg *Config) (err error) {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Services == nil {
		cfg.Services = map[string]*Service{}
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	tmp, err := os.CreateTemp(s.paths.ConfigDir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("creating temporary config file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting config file permissions: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("syncing config file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing config file: %w", err)
	}
	if err = os.Rename(tmpName, s.paths.ConfigFile()); err != nil {
		return fmt.Errorf("replacing config file: %w", err)
	}
	return nil
}
