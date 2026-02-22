package events

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventTypeSessionStarted   EventType = "SESSION_STARTED"
	EventTypeSessionEnded     EventType = "SESSION_ENDED"
	EventTypeMeterSample      EventType = "METER_SAMPLE"
	EventTypeChargingState    EventType = "CHARGING_STATE"
	EventTypeTariffSnapshot   EventType = "TARIFF_SNAPSHOT"
	EventTypeReservationEvent EventType = "RESERVATION_EVENT"
)

type Event struct {
	EventID       string
	SessionID     string
	EventTime     time.Time
	IngestTime    time.Time
	Type          EventType
	Payload       json.RawMessage
	SchemaVersion string
}

func TypeTieBreaker(t EventType) int {
	switch t {
	case EventTypeSessionStarted:
		return 10
	case EventTypeTariffSnapshot:
		return 20
	case EventTypeReservationEvent:
		return 30
	case EventTypeChargingState:
		return 40
	case EventTypeMeterSample:
		return 50
	case EventTypeSessionEnded:
		return 60
	default:
		return 1000
	}
}

func (e Event) normalized() Event {
	e.EventTime = e.EventTime.UTC()
	e.IngestTime = e.IngestTime.UTC()
	if e.Payload != nil {
		e.Payload = append(json.RawMessage(nil), e.Payload...)
	}
	return e
}
