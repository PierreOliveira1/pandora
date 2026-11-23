package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	fileLockTimeout = 10 * time.Second
	fileLockRetry   = 50 * time.Millisecond
)

type fileStore struct {
	path string
}

func NewFileStore(path string) Store {
	return &fileStore{path: path}
}

func (s *fileStore) Get(key string) (string, error) {
	values, err := s.load()
	if err != nil {
		return "", err
	}
	value, ok := values[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *fileStore) Set(key, value string) error {
	return s.update(func(values map[string]string) bool {
		values[key] = value
		return true
	})
}

func (s *fileStore) Delete(key string) error {
	return s.update(func(values map[string]string) bool {
		if _, ok := values[key]; !ok {
			return false
		}
		delete(values, key)
		return true
	})
}

func (s *fileStore) update(mutate func(map[string]string) bool) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}

	lock := flock.New(s.path + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), fileLockTimeout)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, fileLockRetry)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("locking secrets file: timed out waiting for another process")
		}
		return fmt.Errorf("locking secrets file: %w", err)
	}
	if !locked {
		return fmt.Errorf("locking secrets file: timed out waiting for another process")
	}
	defer func() {
		_ = lock.Unlock()
	}()

	values, err := s.load()
	if err != nil {
		return err
	}
	if !mutate(values) {
		return nil
	}
	return s.write(values)
}

func (s *fileStore) load() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading secrets file: %w", err)
	}

	values := map[string]string{}
	if len(data) == 0 {
		return values, nil
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parsing secrets file %s: %w", s.path, err)
	}
	if values == nil {
		values = map[string]string{}
	}
	return values, nil
}

func (s *fileStore) write(values map[string]string) (err error) {
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encoding secrets: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".secrets-*.json")
	if err != nil {
		return fmt.Errorf("creating temporary secrets file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting secrets file permissions: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing secrets file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("syncing secrets file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing secrets file: %w", err)
	}
	if err = os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replacing secrets file: %w", err)
	}
	return nil
}
