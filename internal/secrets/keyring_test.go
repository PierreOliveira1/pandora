package secrets

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

type limitedKeyring struct {
	items     map[string]string
	limit     int
	failAfter int
	sets      int
}

func newLimitedKeyring() *limitedKeyring {
	return &limitedKeyring{items: map[string]string{}, limit: 3000, failAfter: -1}
}

func (k *limitedKeyring) Get(service, user string) (string, error) {
	value, ok := k.items[service+"|"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (k *limitedKeyring) Set(service, user, password string) error {
	if k.failAfter >= 0 && k.sets >= k.failAfter {
		return errors.New("keychain is locked")
	}
	k.sets++
	if len(service)+len(user)+len(password) > k.limit {
		return keyring.ErrSetDataTooBig
	}
	k.items[service+"|"+user] = password
	return nil
}

func (k *limitedKeyring) Delete(service, user string) error {
	if _, ok := k.items[service+"|"+user]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.items, service+"|"+user)
	return nil
}

func largeToken() string {
	return strings.Repeat("eyJhbGciOiJSUzI1NiJ9.", 400) + "ç"
}

func TestASecretLargerThanTheKeychainLimitIsSavedAndReadBack(t *testing.T) {
	store := keyringStore{backend: newLimitedKeyring()}
	token := largeToken()

	if err := store.Set("posthog/trono/oauth", token); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := store.Get("posthog/trono/oauth")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != token {
		t.Fatalf("Get() returned %d bytes, want the %d bytes that were saved", len(got), len(token))
	}
}

func TestASecretSavedBeforeSplittingStillReads(t *testing.T) {
	backend := newLimitedKeyring()
	backend.items[keyringService+"|instagram/trono/env/META_TOKEN"] = "abc"
	store := keyringStore{backend: backend}

	got, err := store.Get("instagram/trono/env/META_TOKEN")
	if err != nil || got != "abc" {
		t.Fatalf("Get() = %q, %v; want abc", got, err)
	}
}

func TestReplacingOrDeletingALargeSecretLeavesNoPartsBehind(t *testing.T) {
	backend := newLimitedKeyring()
	store := keyringStore{backend: backend}
	if err := store.Set("posthog/trono/oauth", largeToken()); err != nil {
		t.Fatalf("Set() large error = %v", err)
	}
	if err := store.Set("posthog/trono/oauth", "small"); err != nil {
		t.Fatalf("Set() small error = %v", err)
	}
	if len(backend.items) != 1 {
		t.Fatalf("keychain has %d items after replacing with a small secret, want 1", len(backend.items))
	}

	if err := store.Set("posthog/trono/oauth", largeToken()); err != nil {
		t.Fatalf("Set() large again error = %v", err)
	}
	if err := store.Delete("posthog/trono/oauth"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(backend.items) != 0 {
		t.Fatalf("keychain has %d items after delete, want 0", len(backend.items))
	}
}

func TestAFailedWriteKeepsThePreviousSecret(t *testing.T) {
	backend := newLimitedKeyring()
	store := keyringStore{backend: backend}
	previous := largeToken()
	if err := store.Set("posthog/trono/oauth", previous); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	stored := len(backend.items)

	backend.failAfter = backend.sets + 2
	if err := store.Set("posthog/trono/oauth", strings.Repeat("x", 9000)); err == nil {
		t.Fatal("Set() error = nil, want the keychain error")
	}

	got, err := store.Get("posthog/trono/oauth")
	if err != nil || got != previous {
		t.Fatalf("Get() after a failed write returned %d bytes, %v; want the previous secret", len(got), err)
	}
	if len(backend.items) != stored {
		t.Fatalf("keychain has %d items after a failed write, want %d", len(backend.items), stored)
	}
}
