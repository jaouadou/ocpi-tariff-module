package periods

import (
	"fmt"
	"sort"
	"time"

	"github.com/jaouadou/ocpi-tariff-module/internal/breakpoints"
	"github.com/jaouadou/ocpi-tariff-module/internal/tariffs"
)

// TraceReason describes why a breakpoint was created.
type TraceReason string

const (
	TraceReasonMeterSampleBoundary     TraceReason = "meter_sample_boundary"
	TraceReasonCalendarBoundary        TraceReason = "calendar_boundary"
	TraceReasonEnergyThresholdCrossing TraceReason = "energy_threshold_crossing"
	TraceReasonTelemetrySampleBoundary TraceReason = "telemetry_sample_boundary"
	TraceReasonTariffSwitch            TraceReason = "tariff_switch"
	TraceReasonChargingToParking       TraceReason = "charging_to_parking"
	TraceReasonMeterRollback           TraceReason = "meter_rollback"
)

// TraceEvent represents a single trace entry describing why a period boundary occurred.
type TraceEvent struct {
	At        time.Time
	Reason    TraceReason
	Detail    string
	PeriodKey string
}

// Trace collects trace events during accumulation.
// When nil, tracing is disabled.
type Trace struct {
	Events []TraceEvent
}

// AddEvent records a trace event if tracing is enabled.
func (t *Trace) AddEvent(at time.Time, reason TraceReason, detail string, periodKey string) {
	if t == nil {
		return
	}
	t.Events = append(t.Events, TraceEvent{
		At:        at,
		Reason:    reason,
		Detail:    detail,
		PeriodKey: periodKey,
	})
}

// AccumulateWithTrace is like Accumulate but also records trace events describing
// why each period boundary was created.
func AccumulateWithTrace(
	startUTC, endUTC time.Time,
	tariff tariffs.Tariff,
	meter []breakpoints.MeterSample,
	power []breakpoints.PowerSample,
	currentSamples []breakpoints.CurrentSample,
	calendar []time.Time,
	thresholds []breakpoints.EnergyThreshold,
	trace *Trace,
) ([]ChargingPeriod, error) {
	if !startUTC.Before(endUTC) {
		return nil, fmt.Errorf("invalid session window")
	}
	if len(meter) == 0 {
		return nil, fmt.Errorf("at least one meter sample is required")
	}

	startUTC = startUTC.UTC()
	endUTC = endUTC.UTC()

	normalized := normalizeMeterSamples(meter)
	normalizedPower := normalizePowerSamples(power)
	normalizedCurrent := normalizeCurrentSamples(currentSamples)
	startTotalKWh, err := totalKWhAt(normalized, startUTC)
	if err != nil {
		return nil, err
	}

	bps := breakpoints.Breakpoints(startUTC, endUTC, normalized, calendar, thresholds)
	boundariesByNanos := make(map[int64]time.Time, len(bps)+len(normalizedPower)+len(normalizedCurrent)+2)
	addBoundary(boundariesByNanos, startUTC)
	for _, bp := range bps {
		addBoundary(boundariesByNanos, bp)
	}
	for _, sample := range normalizedPower {
		addBoundary(boundariesByNanos, sample.At)
	}
	for _, sample := range normalizedCurrent {
		addBoundary(boundariesByNanos, sample.At)
	}
	addBoundary(boundariesByNanos, endUTC)

	boundaries := make([]time.Time, 0, len(boundariesByNanos))
	for _, boundary := range boundariesByNanos {
		if boundary.Before(startUTC) || boundary.After(endUTC) {
			continue
		}
		boundaries = append(boundaries, boundary)
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Before(boundaries[j]) })

	periods := make([]ChargingPeriod, 0)
	var current *ChargingPeriod
	var prevSelected map[tariffs.TariffDimensionType]tariffs.TariffElement
	var prevCharging bool
	currentKey := ""

	// Record initial state
	if trace != nil {
		trace.AddEvent(startUTC, TraceReasonMeterSampleBoundary, "session start", "")
	}

	for i := 1; i < len(boundaries); i++ {
		a := boundaries[i-1]
		b := boundaries[i]
		if !a.Before(b) {
			continue
		}

		energyStart, err := totalKWhAt(normalized, a)
		if err != nil {
			return nil, err
		}
		energyEnd, err := totalKWhAt(normalized, b)
		if err != nil {
			return nil, err
		}

		deltaKWh := energyEnd - energyStart
		if deltaKWh < 0 {
			if trace != nil {
				detail := fmt.Sprintf("meter dropped from %.6f to %.6f kWh", energyStart, energyEnd)
				trace.AddEvent(b, TraceReasonMeterRollback, detail, currentKey)
			}
			deltaKWh = 0
		}

		sessionEnergyAtA := energyStart - startTotalKWh
		if sessionEnergyAtA < 0 {
			sessionEnergyAtA = 0
		}

		snap := tariffs.Snapshot{
			At:           a,
			EnergyKWh:    sessionEnergyAtA,
			Duration:     a.Sub(startUTC),
			PowerKW:      powerAt(normalizedPower, a),
			PowerKnown:   powerKnownAt(normalizedPower, a),
			CurrentA:     currentAt(normalizedCurrent, a),
			CurrentKnown: currentKnownAt(normalizedCurrent, a),
		}
		selected := tariffs.SelectActiveElements(tariff, snap)

		charging := deltaKWh > 0
		key := selectionKey(selected, charging)

		if current == nil || key != currentKey {
			// Determine why the split occurred
			if trace != nil && prevSelected != nil {
				recordBreakpointReason(trace, a, prevSelected, prevCharging, selected, charging, key)
			}

			periods = append(periods, ChargingPeriod{Start: a.UTC()})
			current = &periods[len(periods)-1]
			currentKey = key
		}

		if charging {
			addDimension(current, DimensionTypeEnergy, deltaKWh)
			addDimension(current, DimensionTypeTime, b.Sub(a).Hours())
		} else {
			addDimension(current, DimensionTypeParkingTime, b.Sub(a).Hours())
		}

		prevSelected = selected
		prevCharging = charging
	}

	for i := range periods {
		periods[i].Dimensions = normalizeDimensions(periods[i].Dimensions)
	}

	return periods, nil
}

// recordBreakpointReason analyzes what caused the period split and records a trace event.
func recordBreakpointReason(
	trace *Trace,
	at time.Time,
	prevSelected map[tariffs.TariffDimensionType]tariffs.TariffElement,
	prevCharging bool,
	currentSelected map[tariffs.TariffDimensionType]tariffs.TariffElement,
	charging bool,
	currentKey string,
) {
	// Check for tariff element change
	if !mapsEqual(prevSelected, currentSelected) {
		var detail string
		if prevSelected == nil {
			detail = "tariff element selected"
		} else {
			detail = describeTariffDiff(prevSelected, currentSelected)
		}
		trace.AddEvent(at, TraceReasonTariffSwitch, detail, currentKey)
		return
	}

	// Check for charging to parking transition
	if prevCharging && !charging {
		trace.AddEvent(at, TraceReasonChargingToParking, "energy consumption stopped", currentKey)
		return
	}

	// Default fallback
	trace.AddEvent(at, TraceReasonMeterSampleBoundary, "boundary point", currentKey)
}

// mapsEqual compares two tariff element maps.
func mapsEqual(a, b map[tariffs.TariffDimensionType]tariffs.TariffElement) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k].ID != v.ID {
			return false
		}
	}
	return true
}

// describeTariffDiff creates a human-readable description of what tariff elements changed.
func describeTariffDiff(prev, current map[tariffs.TariffDimensionType]tariffs.TariffElement) string {
	var changes []string
	for dimType, currElem := range current {
		prevElem, exists := prev[dimType]
		if !exists {
			changes = append(changes, fmt.Sprintf("%s: none -> %s", dimType, currElem.ID))
		} else if prevElem.ID != currElem.ID {
			changes = append(changes, fmt.Sprintf("%s: %s -> %s", dimType, prevElem.ID, currElem.ID))
		}
	}
	// Check for removed dimensions
	for dimType, prevElem := range prev {
		if _, exists := current[dimType]; !exists {
			changes = append(changes, fmt.Sprintf("%s: %s -> none", dimType, prevElem.ID))
		}
	}
	if len(changes) == 0 {
		return "tariff element changed"
	}
	return "tariff element became active"
}
