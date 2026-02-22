package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventOrdering_ShuffleArrival(t *testing.T) {
	t0 := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	sessionID := "sess-1"

	canonical := []Event{
		{EventID: "evt-003", SessionID: sessionID, EventTime: t0, IngestTime: t0.Add(1 * time.Second), Type: EventTypeSessionStarted, Payload: json.RawMessage(`{"start_time":"2026-02-22T10:00:00Z"}`), SchemaVersion: "1"},
		{EventID: "evt-001", SessionID: sessionID, EventTime: t0.Add(30 * time.Second), IngestTime: t0.Add(31 * time.Second), Type: EventTypeTariffSnapshot, Payload: json.RawMessage(`{"tariff_id":"T-1"}`), SchemaVersion: "1"},
		{EventID: "evt-005", SessionID: sessionID, EventTime: t0.Add(30 * time.Second), IngestTime: t0.Add(33 * time.Second), Type: EventTypeReservationEvent, Payload: json.RawMessage(`{"state":"created"}`), SchemaVersion: "1"},
		{EventID: "evt-002", SessionID: sessionID, EventTime: t0.Add(30 * time.Second), IngestTime: t0.Add(32 * time.Second), Type: EventTypeChargingState, Payload: json.RawMessage(`{"state":"CHARGING"}`), SchemaVersion: "1"},
		{EventID: "evt-010", SessionID: sessionID, EventTime: t0.Add(30 * time.Second), IngestTime: t0.Add(34 * time.Second), Type: EventTypeMeterSample, Payload: json.RawMessage(`{"meter_kwh_total":12.3}`), SchemaVersion: "1"},
		{EventID: "evt-099", SessionID: sessionID, EventTime: t0.Add(2 * time.Minute), IngestTime: t0.Add(121 * time.Second), Type: EventTypeSessionEnded, Payload: json.RawMessage(`{"reason":"remote"}`), SchemaVersion: "1"},
	}

	arrivalA := []Event{canonical[4], canonical[0], canonical[2], canonical[5], canonical[1], canonical[3]}
	arrivalB := []Event{canonical[1], canonical[5], canonical[3], canonical[0], canonical[4], canonical[2]}

	storeA := NewStore()
	storeB := NewStore()

	for _, e := range arrivalA {
		added, quarantined := storeA.Add(e)
		require.True(t, added)
		require.False(t, quarantined)
	}

	for _, e := range arrivalB {
		added, quarantined := storeB.Add(e)
		require.True(t, added)
		require.False(t, quarantined)
	}

	added, quarantined := storeA.Add(Event{
		EventID:       "evt-010",
		SessionID:     sessionID,
		EventTime:     t0.Add(30 * time.Second),
		IngestTime:    t0.Add(45 * time.Second),
		Type:          EventTypeMeterSample,
		Payload:       json.RawMessage(`{"meter_kwh_total":99.9}`),
		SchemaVersion: "1",
	})
	require.False(t, added)
	require.False(t, quarantined)

	orderedA := storeA.Ordered(sessionID)
	orderedB := storeB.Ordered(sessionID)

	require.Equal(t, orderedA, orderedB)
	require.Equal(t, []string{"evt-003", "evt-001", "evt-005", "evt-002", "evt-010", "evt-099"}, eventIDs(orderedA))
	require.Equal(t, t0, storeA.Watermark(sessionID))
	require.Equal(t, t0, storeB.Watermark(sessionID))
}

func TestTypeTieBreaker_IsDeterministic(t *testing.T) {
	require.Equal(t, 10, TypeTieBreaker(EventTypeSessionStarted))
	require.Equal(t, 20, TypeTieBreaker(EventTypeTariffSnapshot))
	require.Equal(t, 30, TypeTieBreaker(EventTypeReservationEvent))
	require.Equal(t, 40, TypeTieBreaker(EventTypeChargingState))
	require.Equal(t, 50, TypeTieBreaker(EventTypeMeterSample))
	require.Equal(t, 60, TypeTieBreaker(EventTypeSessionEnded))
	require.Equal(t, 1000, TypeTieBreaker(EventType("UNKNOWN_TYPE")))
}

func TestStore_QuarantinesExtremeLateEvent(t *testing.T) {
	store := NewStore()
	sessionID := "sess-q"
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	added, quarantined := store.Add(Event{
		EventID:       "evt-new",
		SessionID:     sessionID,
		EventTime:     base.Add(72 * time.Hour),
		IngestTime:    base.Add(72 * time.Hour),
		Type:          EventTypeSessionStarted,
		Payload:       json.RawMessage(`{}`),
		SchemaVersion: "1",
	})
	require.True(t, added)
	require.False(t, quarantined)

	added, quarantined = store.Add(Event{
		EventID:       "evt-too-late",
		SessionID:     sessionID,
		EventTime:     base,
		IngestTime:    base.Add(73 * time.Hour),
		Type:          EventTypeMeterSample,
		Payload:       json.RawMessage(`{"meter_kwh_total":1}`),
		SchemaVersion: "1",
	})
	require.True(t, added)
	require.True(t, quarantined)

	ordered := store.Ordered(sessionID)
	require.Len(t, ordered, 1)
	require.Equal(t, "evt-new", ordered[0].EventID)
}

func TestStore_QuarantinesWhenActiveCapExceeded(t *testing.T) {
	store := NewStore()
	store.maxActiveEventsPerSession = 2
	sessionID := "sess-cap"
	base := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 2; i++ {
		added, quarantined := store.Add(Event{
			EventID:       "evt-ok-" + string(rune('a'+i)),
			SessionID:     sessionID,
			EventTime:     base.Add(time.Duration(i) * time.Second),
			IngestTime:    base.Add(time.Duration(i) * time.Second),
			Type:          EventTypeMeterSample,
			Payload:       json.RawMessage(`{"meter_kwh_total":1}`),
			SchemaVersion: "1",
		})
		require.True(t, added)
		require.False(t, quarantined)
	}

	added, quarantined := store.Add(Event{
		EventID:       "evt-overflow",
		SessionID:     sessionID,
		EventTime:     base.Add(3 * time.Second),
		IngestTime:    base.Add(3 * time.Second),
		Type:          EventTypeMeterSample,
		Payload:       json.RawMessage(`{"meter_kwh_total":2}`),
		SchemaVersion: "1",
	})
	require.True(t, added)
	require.True(t, quarantined)

	ordered := store.Ordered(sessionID)
	require.Len(t, ordered, 2)
	require.Equal(t, []string{"evt-ok-a", "evt-ok-b"}, eventIDs(ordered))

	added, quarantined = store.Add(Event{
		EventID:       "evt-overflow",
		SessionID:     sessionID,
		EventTime:     base.Add(4 * time.Second),
		IngestTime:    base.Add(4 * time.Second),
		Type:          EventTypeMeterSample,
		Payload:       json.RawMessage(`{"meter_kwh_total":3}`),
		SchemaVersion: "1",
	})
	require.False(t, added)
	require.False(t, quarantined)
}

func eventIDs(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.EventID)
	}
	return out
}
