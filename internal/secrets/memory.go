package secrets

import "sync"

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemoryStore() Store {
	return &memoryStore{values: map[string]string{}}
}

func (s *memoryStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *memoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *memoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}
