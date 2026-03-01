package httpapi

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

var ErrSessionExists = errors.New("session already exists")
var ErrSessionNotFound = errors.New("session not found")
var ErrSessionAlreadyEnded = errors.New("session already ended")

type SessionState struct {
	ID       uuid.UUID
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
	requests chan sessionStoreRequest
}

type identifiedSample[T any] struct {
	id     string
	sample T
}

type identifiedMeterSample = identifiedSample[segengine.MeterSample]
type identifiedPowerSample = identifiedSample[segengine.PowerSample]
type identifiedCurrentSample = identifiedSample[segengine.CurrentSample]

type sessionStoreData struct {
	sessions map[uuid.UUID]*SessionState
}

type sessionStoreRequest struct {
	run func(*sessionStoreData)
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
	store := &SessionStore{requests: make(chan sessionStoreRequest)}
	data := &sessionStoreData{sessions: make(map[uuid.UUID]*SessionState)}

	go func() {
		for req := range store.requests {
			req.run(data)
		}
	}()

	return store
}

func (s *SessionStore) CreateSession(state *SessionState) error {
	result := make(chan error, 1)
	s.requests <- sessionStoreRequest{run: func(data *sessionStoreData) {
		if _, exists := data.sessions[state.ID]; exists {
			result <- ErrSessionExists
			return
		}

		data.sessions[state.ID] = state
		result <- nil
	}}

	return <-result
}

func (s *SessionStore) AppendMeterSamples(id uuid.UUID, samples []identifiedMeterSample) (accepted, duplicates int, err error) {
	return appendSessionSamples(s, id, samples, func(state *SessionState) map[string]segengine.MeterSample {
		return state.MeterSamples
	})
}

func (s *SessionStore) AppendPowerSamples(id uuid.UUID, samples []identifiedPowerSample) (accepted, duplicates int, err error) {
	return appendSessionSamples(s, id, samples, func(state *SessionState) map[string]segengine.PowerSample {
		return state.PowerSamples
	})
}

func (s *SessionStore) AppendCurrentSamples(id uuid.UUID, samples []identifiedCurrentSample) (accepted, duplicates int, err error) {
	return appendSessionSamples(s, id, samples, func(state *SessionState) map[string]segengine.CurrentSample {
		return state.CurrentSamples
	})
}

func (s *SessionStore) SnapshotSession(id uuid.UUID) (SessionSnapshot, error) {
	result := make(chan struct {
		snapshot SessionSnapshot
		err      error
	}, 1)

	s.requests <- sessionStoreRequest{run: func(data *sessionStoreData) {
		state, ok := data.sessions[id]
		if !ok {
			result <- struct {
				snapshot SessionSnapshot
				err      error
			}{err: ErrSessionNotFound}
			return
		}

		result <- struct {
			snapshot SessionSnapshot
			err      error
		}{snapshot: snapshotFromSessionState(state)}
	}}

	out := <-result
	return out.snapshot, out.err
}

func (s *SessionStore) EndSession(id uuid.UUID, endUTC time.Time) (SessionSnapshot, error) {
	result := make(chan struct {
		snapshot SessionSnapshot
		err      error
	}, 1)

	s.requests <- sessionStoreRequest{run: func(data *sessionStoreData) {
		state, ok := data.sessions[id]
		if !ok {
			result <- struct {
				snapshot SessionSnapshot
				err      error
			}{err: ErrSessionNotFound}
			return
		}

		if state.EndUTC != nil {
			result <- struct {
				snapshot SessionSnapshot
				err      error
			}{err: ErrSessionAlreadyEnded}
			return
		}

		endedAt := endUTC.UTC()
		state.EndUTC = &endedAt

		result <- struct {
			snapshot SessionSnapshot
			err      error
		}{snapshot: snapshotFromSessionState(state)}
	}}

	out := <-result
	return out.snapshot, out.err
}

func appendSessionSamples[T any](
	store *SessionStore,
	id uuid.UUID,
	samples []identifiedSample[T],
	selectTarget func(*SessionState) map[string]T,
) (accepted, duplicates int, err error) {
	result := make(chan struct {
		accepted   int
		duplicates int
		err        error
	}, 1)

	store.requests <- sessionStoreRequest{run: func(data *sessionStoreData) {
		state, ok := data.sessions[id]
		if !ok {
			result <- struct {
				accepted   int
				duplicates int
				err        error
			}{err: ErrSessionNotFound}
			return
		}

		acceptedCount, duplicateCount := appendIdentifiedSamples(selectTarget(state), samples)
		result <- struct {
			accepted   int
			duplicates int
			err        error
		}{accepted: acceptedCount, duplicates: duplicateCount}
	}}

	out := <-result
	return out.accepted, out.duplicates, out.err
}

func appendIdentifiedSamples[T any](target map[string]T, samples []identifiedSample[T]) (accepted, duplicates int) {
	for _, item := range samples {
		if _, exists := target[item.id]; exists {
			duplicates++
			continue
		}

		target[item.id] = item.sample
		accepted++
	}

	return accepted, duplicates
}

func snapshotFromSessionState(state *SessionState) SessionSnapshot {
	var endUTC *time.Time
	if state.EndUTC != nil {
		copied := state.EndUTC.UTC()
		endUTC = &copied
	}

	return SessionSnapshot{
		ID:             state.ID.String(),
		StartUTC:       state.StartUTC,
		EndUTC:         endUTC,
		Location:       state.Location,
		Tariff:         state.Tariff,
		MeterSamples:   snapshotMeterSamples(state.MeterSamples),
		PowerSamples:   snapshotPowerSamples(state.PowerSamples),
		CurrentSamples: snapshotCurrentSamples(state.CurrentSamples),
	}
}

func snapshotMeterSamples(source map[string]segengine.MeterSample) []segengine.MeterSample {
	return snapshotSamples(source, func(sample segengine.MeterSample) time.Time {
		return sample.At
	})
}

func snapshotPowerSamples(source map[string]segengine.PowerSample) []segengine.PowerSample {
	return snapshotSamples(source, func(sample segengine.PowerSample) time.Time {
		return sample.At
	})
}

func snapshotCurrentSamples(source map[string]segengine.CurrentSample) []segengine.CurrentSample {
	return snapshotSamples(source, func(sample segengine.CurrentSample) time.Time {
		return sample.At
	})
}

func snapshotSamples[T any](source map[string]T, at func(T) time.Time) []T {
	type entry struct {
		id     string
		sample T
	}

	entries := make([]entry, 0, len(source))
	for id, sample := range source {
		entries = append(entries, entry{id: id, sample: sample})
	}

	sort.Slice(entries, func(i, j int) bool {
		leftAt := at(entries[i].sample)
		rightAt := at(entries[j].sample)
		if !leftAt.Equal(rightAt) {
			return leftAt.Before(rightAt)
		}
		return entries[i].id < entries[j].id
	})

	out := make([]T, 0, len(entries))
	for _, item := range entries {
		out = append(out, item.sample)
	}
	return out
}
