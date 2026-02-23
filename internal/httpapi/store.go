package httpapi

import (
	"errors"
	"sort"
	"sync"
	"time"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

var ErrSessionExists = errors.New("session already exists")
var ErrSessionNotFound = errors.New("session not found")
var ErrSessionAlreadyEnded = errors.New("session already ended")

type SessionState struct {
	mu sync.Mutex

	ID       string
	StartUTC time.Time
	EndUTC   *time.Time

	Timezone string
	Location *time.Location
	Tariff   segengine.Tariff

	MeterSamples   map[string]segengine.MeterSample
	PowerSamples   map[string]segengine.PowerSample
	CurrentSamples map[string]segengine.CurrentSample
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
}

type identifiedMeterSample struct {
	id     string
	sample segengine.MeterSample
}

type identifiedPowerSample struct {
	id     string
	sample segengine.PowerSample
}

type identifiedCurrentSample struct {
	id     string
	sample segengine.CurrentSample
}

type SessionSnapshot struct {
	ID       string
	StartUTC time.Time
	EndUTC   *time.Time

	Location *time.Location
	Tariff   segengine.Tariff

	MeterSamples   []segengine.MeterSample
	PowerSamples   []segengine.PowerSample
	CurrentSamples []segengine.CurrentSample
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*SessionState)}
}

func (s *SessionStore) CreateSession(state *SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[state.ID]; exists {
		return ErrSessionExists
	}

	s.sessions[state.ID] = state
	return nil
}

func (s *SessionStore) Session(id string) (*SessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.sessions[id]
	return state, ok
}

func (s *SessionStore) AppendMeterSamples(id string, samples []identifiedMeterSample) (accepted, duplicates int, err error) {
	state, ok := s.Session(id)
	if !ok {
		return 0, 0, ErrSessionNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	for _, item := range samples {
		if _, exists := state.MeterSamples[item.id]; exists {
			duplicates++
			continue
		}
		state.MeterSamples[item.id] = item.sample
		accepted++
	}

	return accepted, duplicates, nil
}

func (s *SessionStore) AppendPowerSamples(id string, samples []identifiedPowerSample) (accepted, duplicates int, err error) {
	state, ok := s.Session(id)
	if !ok {
		return 0, 0, ErrSessionNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	for _, item := range samples {
		if _, exists := state.PowerSamples[item.id]; exists {
			duplicates++
			continue
		}
		state.PowerSamples[item.id] = item.sample
		accepted++
	}

	return accepted, duplicates, nil
}

func (s *SessionStore) AppendCurrentSamples(id string, samples []identifiedCurrentSample) (accepted, duplicates int, err error) {
	state, ok := s.Session(id)
	if !ok {
		return 0, 0, ErrSessionNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	for _, item := range samples {
		if _, exists := state.CurrentSamples[item.id]; exists {
			duplicates++
			continue
		}
		state.CurrentSamples[item.id] = item.sample
		accepted++
	}

	return accepted, duplicates, nil
}

func (s *SessionStore) SnapshotSession(id string) (SessionSnapshot, error) {
	state, ok := s.Session(id)
	if !ok {
		return SessionSnapshot{}, ErrSessionNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	var endUTC *time.Time
	if state.EndUTC != nil {
		copied := state.EndUTC.UTC()
		endUTC = &copied
	}

	return SessionSnapshot{
		ID:             state.ID,
		StartUTC:       state.StartUTC,
		EndUTC:         endUTC,
		Location:       state.Location,
		Tariff:         state.Tariff,
		MeterSamples:   snapshotMeterSamples(state.MeterSamples),
		PowerSamples:   snapshotPowerSamples(state.PowerSamples),
		CurrentSamples: snapshotCurrentSamples(state.CurrentSamples),
	}, nil
}

func (s *SessionStore) EndSession(id string, endUTC time.Time) (SessionSnapshot, error) {
	state, ok := s.Session(id)
	if !ok {
		return SessionSnapshot{}, ErrSessionNotFound
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.EndUTC != nil {
		return SessionSnapshot{}, ErrSessionAlreadyEnded
	}

	endedAt := endUTC.UTC()
	state.EndUTC = &endedAt

	endCopy := state.EndUTC.UTC()

	return SessionSnapshot{
		ID:             state.ID,
		StartUTC:       state.StartUTC,
		EndUTC:         &endCopy,
		Location:       state.Location,
		Tariff:         state.Tariff,
		MeterSamples:   snapshotMeterSamples(state.MeterSamples),
		PowerSamples:   snapshotPowerSamples(state.PowerSamples),
		CurrentSamples: snapshotCurrentSamples(state.CurrentSamples),
	}, nil
}

func snapshotMeterSamples(source map[string]segengine.MeterSample) []segengine.MeterSample {
	type entry struct {
		id     string
		sample segengine.MeterSample
	}

	entries := make([]entry, 0, len(source))
	for id, sample := range source {
		entries = append(entries, entry{id: id, sample: sample})
	}

	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].sample.At.Equal(entries[j].sample.At) {
			return entries[i].sample.At.Before(entries[j].sample.At)
		}
		return entries[i].id < entries[j].id
	})

	out := make([]segengine.MeterSample, 0, len(entries))
	for _, item := range entries {
		out = append(out, item.sample)
	}
	return out
}

func snapshotPowerSamples(source map[string]segengine.PowerSample) []segengine.PowerSample {
	type entry struct {
		id     string
		sample segengine.PowerSample
	}

	entries := make([]entry, 0, len(source))
	for id, sample := range source {
		entries = append(entries, entry{id: id, sample: sample})
	}

	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].sample.At.Equal(entries[j].sample.At) {
			return entries[i].sample.At.Before(entries[j].sample.At)
		}
		return entries[i].id < entries[j].id
	})

	out := make([]segengine.PowerSample, 0, len(entries))
	for _, item := range entries {
		out = append(out, item.sample)
	}
	return out
}

func snapshotCurrentSamples(source map[string]segengine.CurrentSample) []segengine.CurrentSample {
	type entry struct {
		id     string
		sample segengine.CurrentSample
	}

	entries := make([]entry, 0, len(source))
	for id, sample := range source {
		entries = append(entries, entry{id: id, sample: sample})
	}

	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].sample.At.Equal(entries[j].sample.At) {
			return entries[i].sample.At.Before(entries[j].sample.At)
		}
		return entries[i].id < entries[j].id
	})

	out := make([]segengine.CurrentSample, 0, len(entries))
	for _, item := range entries {
		out = append(out, item.sample)
	}
	return out
}
