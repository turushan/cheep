package secrets

import "sync"

// Memory is a test store that never touches an operating system credential manager.
type Memory struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemory() *Memory {
	return &Memory{values: make(map[string]string)}
}

func (m *Memory) Get(profile string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.values[profile]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (m *Memory) Set(profile, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[profile] = value
	return nil
}

func (m *Memory) Delete(profile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.values[profile]; !ok {
		return ErrNotFound
	}
	delete(m.values, profile)
	return nil
}
