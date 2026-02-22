package events

import (
	"slices"
	"sync"
	"time"
)

const defaultAllowedLateness = 120 * time.Second
const defaultMaxActiveEventsPerSession = 10_000

type Store struct {
	mu                        sync.RWMutex
	allowedLateness           time.Duration
	maxActiveEventsPerSession int
	sessions                  map[string]*sessionStore
}

type sessionStore struct {
	seen        map[string]struct{}
	active      []Event
	quarantine  []Event
	maxEventUTC time.Time
	hasMaxEvent bool
}

func NewStore() *Store {
	return &Store{
		allowedLateness:           defaultAllowedLateness,
		maxActiveEventsPerSession: defaultMaxActiveEventsPerSession,
		sessions:                  make(map[string]*sessionStore),
	}
}

func (s *Store) Add(e Event) (added bool, quarantined bool) {
	e = e.normalized()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureInitializedLocked()
	if e.EventID == "" || e.SessionID == "" {
		return false, false
	}

	ss := s.ensureSessionLocked(e.SessionID)
	if _, ok := ss.seen[e.EventID]; ok {
		return false, false
	}

	if s.isTooLateLocked(ss, e.EventTime) {
		ss.seen[e.EventID] = struct{}{}
		ss.quarantine = append(ss.quarantine, e)
		return true, true
	}
	if len(ss.active) >= s.maxActiveEventsPerSession {
		ss.seen[e.EventID] = struct{}{}
		ss.quarantine = append(ss.quarantine, e)
		return true, true
	}

	ss.seen[e.EventID] = struct{}{}
	ss.active = append(ss.active, e)
	if !ss.hasMaxEvent || e.EventTime.After(ss.maxEventUTC) {
		ss.maxEventUTC = e.EventTime
		ss.hasMaxEvent = true
	}

	return true, false
}

func (s *Store) Ordered(sessionID string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ss, ok := s.sessions[sessionID]
	if !ok || len(ss.active) == 0 {
		return nil
	}

	ordered := make([]Event, len(ss.active))
	copy(ordered, ss.active)
	slices.SortFunc(ordered, func(a, b Event) int {
		if !a.EventTime.Equal(b.EventTime) {
			if a.EventTime.Before(b.EventTime) {
				return -1
			}
			return 1
		}

		ta := TypeTieBreaker(a.Type)
		tb := TypeTieBreaker(b.Type)
		if ta != tb {
			if ta < tb {
				return -1
			}
			return 1
		}

		if a.EventID < b.EventID {
			return -1
		}
		if a.EventID > b.EventID {
			return 1
		}
		return 0
	})

	for i := range ordered {
		ordered[i] = ordered[i].normalized()
	}

	return ordered
}

func (s *Store) Watermark(sessionID string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ss, ok := s.sessions[sessionID]
	if !ok || !ss.hasMaxEvent {
		return time.Time{}
	}

	return ss.maxEventUTC.Add(-s.allowedLateness).UTC()
}

func (s *Store) ensureInitializedLocked() {
	if s.allowedLateness <= 0 {
		s.allowedLateness = defaultAllowedLateness
	}
	if s.maxActiveEventsPerSession <= 0 {
		s.maxActiveEventsPerSession = defaultMaxActiveEventsPerSession
	}
	if s.sessions == nil {
		s.sessions = make(map[string]*sessionStore)
	}
}

func (s *Store) ensureSessionLocked(sessionID string) *sessionStore {
	if ss, ok := s.sessions[sessionID]; ok {
		return ss
	}
	ss := &sessionStore{seen: make(map[string]struct{})}
	s.sessions[sessionID] = ss
	return ss
}

func (s *Store) isTooLateLocked(ss *sessionStore, eventTime time.Time) bool {
	if !ss.hasMaxEvent {
		return false
	}
	watermark := ss.maxEventUTC.Add(-s.allowedLateness)
	tooLateBoundary := watermark.Add(-24 * time.Hour)
	return eventTime.Before(tooLateBoundary)
}
