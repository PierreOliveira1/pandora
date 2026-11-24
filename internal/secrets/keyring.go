package secrets

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "pandora"
	chunkPrefix    = "pandora-chunked:v1:"
	chunkSize      = 1500
	maxChunks      = 1000
)

type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

type keyringStore struct {
	backend keyringBackend
}

type chunkRef struct {
	generation int
	count      int
}

func NewKeyringStore() Store {
	return keyringStore{backend: systemKeyring{}}
}

func (s keyringStore) Get(key string) (string, error) {
	value, err := s.read(key)
	if err != nil {
		return "", err
	}
	ref, chunked := parseChunkRef(value)
	if !chunked {
		return value, nil
	}
	var b strings.Builder
	for i := 1; i <= ref.count; i++ {
		part, err := s.read(chunkKey(key, ref.generation, i))
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("reading secret from keyring: part %d of %s is missing", i, key)
		}
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

func (s keyringStore) Set(key, value string) error {
	previous, hadChunks := s.storedChunks(key)
	if len(value) <= chunkSize && !strings.HasPrefix(value, chunkPrefix) {
		if err := s.write(key, value); err != nil {
			return err
		}
		if hadChunks {
			s.deleteChunks(key, previous)
		}
		return nil
	}

	parts := splitChunks(value)
	if len(parts) > maxChunks {
		return errors.New("writing secret to keyring: the secret is too large")
	}
	next := chunkRef{generation: previous.generation + 1, count: len(parts)}
	for i, part := range parts {
		if err := s.write(chunkKey(key, next.generation, i+1), part); err != nil {
			s.deleteChunks(key, chunkRef{generation: next.generation, count: i})
			return err
		}
	}
	if err := s.write(key, formatChunkRef(next)); err != nil {
		s.deleteChunks(key, next)
		return err
	}
	if hadChunks {
		s.deleteChunks(key, previous)
	}
	return nil
}

func (s keyringStore) Delete(key string) error {
	previous, hadChunks := s.storedChunks(key)
	if err := s.backend.Delete(keyringService, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("deleting secret from keyring: %w", err)
	}
	if hadChunks {
		s.deleteChunks(key, previous)
	}
	return nil
}

func (s keyringStore) read(key string) (string, error) {
	value, err := s.backend.Get(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading secret from keyring: %w", err)
	}
	return value, nil
}

func (s keyringStore) write(key, value string) error {
	if err := s.backend.Set(keyringService, key, value); err != nil {
		return fmt.Errorf("writing secret to keyring: %w", err)
	}
	return nil
}

func (s keyringStore) storedChunks(key string) (chunkRef, bool) {
	value, err := s.backend.Get(keyringService, key)
	if err != nil {
		return chunkRef{}, false
	}
	return parseChunkRef(value)
}

func (s keyringStore) deleteChunks(key string, ref chunkRef) {
	for i := 1; i <= ref.count; i++ {
		_ = s.backend.Delete(keyringService, chunkKey(key, ref.generation, i))
	}
}

func parseChunkRef(value string) (chunkRef, bool) {
	rest, ok := strings.CutPrefix(value, chunkPrefix)
	if !ok {
		return chunkRef{}, false
	}
	rawGeneration, rawCount, ok := strings.Cut(rest, ":")
	if !ok {
		return chunkRef{}, false
	}
	generation, err := strconv.Atoi(rawGeneration)
	if err != nil || generation < 1 {
		return chunkRef{}, false
	}
	count, err := strconv.Atoi(rawCount)
	if err != nil || count < 1 || count > maxChunks {
		return chunkRef{}, false
	}
	return chunkRef{generation: generation, count: count}, true
}

func formatChunkRef(ref chunkRef) string {
	return chunkPrefix + strconv.Itoa(ref.generation) + ":" + strconv.Itoa(ref.count)
}

func chunkKey(key string, generation, index int) string {
	return key + "#" + strconv.Itoa(generation) + "." + strconv.Itoa(index)
}

func splitChunks(value string) []string {
	parts := make([]string, 0, len(value)/chunkSize+1)
	for len(value) > chunkSize {
		cut := chunkSize
		for cut > 0 && !isRuneStart(value[cut]) {
			cut--
		}
		parts = append(parts, value[:cut])
		value = value[cut:]
	}
	return append(parts, value)
}

func isRuneStart(b byte) bool {
	return b&0xC0 != 0x80
}
