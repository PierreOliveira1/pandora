package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "pandora"

type keyringStore struct{}

func NewKeyringStore() Store {
	return keyringStore{}
}

func (keyringStore) Get(key string) (string, error) {
	value, err := keyring.Get(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading secret from keyring: %w", err)
	}
	return value, nil
}

func (keyringStore) Set(key, value string) error {
	if err := keyring.Set(keyringService, key, value); err != nil {
		return fmt.Errorf("writing secret to keyring: %w", err)
	}
	return nil
}

func (keyringStore) Delete(key string) error {
	if err := keyring.Delete(keyringService, key); errors.Is(err, keyring.ErrNotFound) {
		return nil
	} else if err != nil {
		return fmt.Errorf("deleting secret from keyring: %w", err)
	}
	return nil
}
