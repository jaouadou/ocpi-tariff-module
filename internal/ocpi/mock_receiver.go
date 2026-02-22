package ocpi

import (
	"context"
	"sync"
)

type MockSessionReceiver struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewMockSessionReceiver() *MockSessionReceiver {
	return &MockSessionReceiver{
		sessions: make(map[string]Session),
	}
}

func (m *MockSessionReceiver) PutSession(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[s.ID] = cloneSession(s)
	return nil
}

func (m *MockSessionReceiver) GetSession(id string) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[id]
	if !ok {
		return Session{}, false
	}

	return cloneSession(s), true
}
