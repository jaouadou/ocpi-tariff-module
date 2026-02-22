package segengine

import (
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/boundaries"
	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
	"github.com/jaouadou/ocpi-tariff-module/internal/events"
	iocpi "github.com/jaouadou/ocpi-tariff-module/internal/ocpi"
	"github.com/jaouadou/ocpi-tariff-module/internal/periods"
	"github.com/jaouadou/ocpi-tariff-module/internal/tariffs"
)

type Tariff = tariffs.Tariff
type TariffElement = tariffs.TariffElement
type TariffRestrictions = tariffs.TariffRestrictions
type PriceComponent = tariffs.PriceComponent
type TariffDimensionType = tariffs.TariffDimensionType
type ReservationRestrictionType = tariffs.ReservationRestrictionType
type Snapshot = tariffs.Snapshot

type MeterSample = breakpoints.MeterSample
type PowerSample = breakpoints.PowerSample
type CurrentSample = breakpoints.CurrentSample
type EnergyThreshold = breakpoints.EnergyThreshold

type ChargingPeriod = periods.ChargingPeriod
type Dimension = periods.Dimension
type DimensionType = periods.DimensionType
type Trace = periods.Trace
type TraceEvent = periods.TraceEvent
type TraceReason = periods.TraceReason

type TariffRestrictionsCalendar = boundaries.TariffRestrictionsCalendar

type Session = iocpi.Session
type SessionReceiver = iocpi.SessionReceiver
type SessionProjector = iocpi.SessionProjector
type MockSessionReceiver = iocpi.MockSessionReceiver
type CDR = iocpi.CDR
type Finalizer = iocpi.Finalizer

type Event = events.Event
type EventType = events.EventType
type EventStore = events.Store

func Matches(r TariffRestrictions, snap Snapshot) bool {
	return tariffs.Matches(r, snap)
}

func SelectActiveElements(t Tariff, snap Snapshot) map[TariffDimensionType]TariffElement {
	return tariffs.SelectActiveElements(t, snap)
}

func CalendarBoundaries(startUTC, endUTC time.Time, loc *time.Location, r TariffRestrictionsCalendar) []time.Time {
	return boundaries.CalendarBoundaries(startUTC, endUTC, loc, r)
}

func BuildBreakpoints(startUTC, endUTC time.Time, meter []MeterSample, calendar []time.Time, thresholds []EnergyThreshold) []time.Time {
	return breakpoints.Breakpoints(startUTC, endUTC, meter, calendar, thresholds)
}

func Accumulate(startUTC, endUTC time.Time, tariff Tariff, meter []MeterSample, power []PowerSample, current []CurrentSample, calendar []time.Time, thresholds []EnergyThreshold) ([]ChargingPeriod, error) {
	return periods.Accumulate(startUTC, endUTC, tariff, meter, power, current, calendar, thresholds)
}

func AccumulateWithTrace(startUTC, endUTC time.Time, tariff Tariff, meter []MeterSample, power []PowerSample, current []CurrentSample, calendar []time.Time, thresholds []EnergyThreshold, trace *Trace) ([]ChargingPeriod, error) {
	return periods.AccumulateWithTrace(startUTC, endUTC, tariff, meter, power, current, calendar, thresholds, trace)
}

func NewSessionProjector(receiver SessionReceiver) *SessionProjector {
	return iocpi.NewSessionProjector(receiver)
}

func NewMockSessionReceiver() *MockSessionReceiver {
	return iocpi.NewMockSessionReceiver()
}

func NewFinalizer() *Finalizer {
	return iocpi.NewFinalizer()
}

func NewEventStore() *EventStore {
	return events.NewStore()
}
