package httpapi

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

func (s *Server) queryPeriodsHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	sessionID, err := uuid.Parse(pathID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_session_id", err.Error())
		return
	}

	snapshot, err := s.store.SnapshotSession(sessionID)
	if err != nil {
		if err == ErrSessionNotFound {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to load session")
		return
	}

	includeTrace, err := parseTraceParam(r.URL.Query().Get("trace"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_trace", "trace must be 0 or 1")
		return
	}

	effectiveEndUTC, err := resolvePeriodsEndUTC(snapshot, r.URL.Query().Get("as_of_utc"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_as_of_utc", "as_of_utc must be RFC3339")
		return
	}

	periodsResponsePayload := periodsResponse{
		SessionID: snapshot.ID,
		StartUTC:  snapshot.StartUTC.Format(time.RFC3339),
		EndUTC:    effectiveEndUTC.Format(time.RFC3339),
		Periods:   make([]chargingPeriodResponse, 0),
	}

	if !snapshot.StartUTC.Before(effectiveEndUTC) {
		if includeTrace {
			periodsResponsePayload.Trace = &traceResponse{Events: []traceEventResponse{}}
		}
		s.writeJSON(w, http.StatusOK, periodsResponsePayload)
		return
	}

	periods, trace, err := computePeriods(snapshot, effectiveEndUTC, includeTrace)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "periods_unavailable", err.Error())
		return
	}

	periodsResponsePayload.Periods = toPeriodsResponse(periods)
	if includeTrace {
		periodsResponsePayload.Trace = &traceResponse{Events: toTraceEventsResponse(trace.Events)}
	}
	s.writeJSON(w, http.StatusOK, periodsResponsePayload)
}

func computePeriods(snapshot SessionSnapshot, effectiveEndUTC time.Time, includeTrace bool) ([]segengine.ChargingPeriod, *segengine.Trace, error) {
	if !snapshot.StartUTC.Before(effectiveEndUTC) {
		return []segengine.ChargingPeriod{}, nil, nil
	}

	normalizedMeter := normalizeMeterSamples(snapshot.MeterSamples)
	calendarBoundaries := collectCalendarBoundaries(snapshot.StartUTC, effectiveEndUTC, snapshot.Location, snapshot.Tariff)
	thresholds := collectEnergyThresholds(snapshot.Tariff)

	if includeTrace {
		trace := &segengine.Trace{}
		periods, err := segengine.AccumulateWithTrace(
			snapshot.StartUTC,
			effectiveEndUTC,
			snapshot.Tariff,
			normalizedMeter,
			snapshot.PowerSamples,
			snapshot.CurrentSamples,
			calendarBoundaries,
			thresholds,
			trace,
		)
		return periods, trace, err
	}

	periods, err := segengine.Accumulate(
		snapshot.StartUTC,
		effectiveEndUTC,
		snapshot.Tariff,
		normalizedMeter,
		snapshot.PowerSamples,
		snapshot.CurrentSamples,
		calendarBoundaries,
		thresholds,
	)
	return periods, nil, err
}

func parseTraceParam(raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "0" {
		return false, nil
	}
	if value == "1" {
		return true, nil
	}
	return false, fmt.Errorf("invalid trace value %q", value)
}

func resolvePeriodsEndUTC(snapshot SessionSnapshot, rawAsOfUTC string) (time.Time, error) {
	asOfUTC := strings.TrimSpace(rawAsOfUTC)
	if asOfUTC != "" {
		parsed, err := time.Parse(time.RFC3339, asOfUTC)
		if err != nil {
			return time.Time{}, err
		}
		return parsed.UTC(), nil
	}

	if snapshot.EndUTC != nil {
		return snapshot.EndUTC.UTC(), nil
	}

	if len(snapshot.MeterSamples) == 0 {
		return snapshot.StartUTC.UTC(), nil
	}

	return snapshot.MeterSamples[len(snapshot.MeterSamples)-1].At.UTC(), nil
}

func normalizeMeterSamples(samples []segengine.MeterSample) []segengine.MeterSample {
	if len(samples) == 0 {
		return nil
	}

	baseline := samples[0].TotalKWh
	out := make([]segengine.MeterSample, 0, len(samples))
	for _, sample := range samples {
		total := math.Max(0, sample.TotalKWh-baseline)
		out = append(out, segengine.MeterSample{At: sample.At.UTC(), TotalKWh: total})
	}

	return out
}

func collectCalendarBoundaries(startUTC, endUTC time.Time, location *time.Location, tariff segengine.Tariff) []time.Time {
	set := make(map[int64]time.Time)
	for _, element := range tariff.Elements {
		elementRestrictionsCalendar := segengine.TariffRestrictionsCalendar{
			StartTime:  element.Restrictions.StartTime,
			EndTime:    element.Restrictions.EndTime,
			StartDate:  element.Restrictions.StartDate,
			EndDate:    element.Restrictions.EndDate,
			DaysOfWeek: element.Restrictions.DayOfWeek,
		}
		for _, boundary := range segengine.CalendarBoundaries(startUTC, endUTC, location, elementRestrictionsCalendar) {
			set[boundary.UnixNano()] = boundary.UTC()
		}
	}

	out := make([]time.Time, 0, len(set))
	for _, boundary := range set {
		out = append(out, boundary)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Before(out[j])
	})

	return out
}

func collectEnergyThresholds(tariff segengine.Tariff) []segengine.EnergyThreshold {
	thresholds := make([]segengine.EnergyThreshold, 0)
	for _, element := range tariff.Elements {
		if element.Restrictions.MinKWh != nil {
			thresholds = append(thresholds, segengine.EnergyThreshold{Kind: "min", KWh: *element.Restrictions.MinKWh})
		}
		if element.Restrictions.MaxKWh != nil {
			thresholds = append(thresholds, segengine.EnergyThreshold{Kind: "max", KWh: *element.Restrictions.MaxKWh})
		}
	}
	return thresholds
}

func toPeriodsResponse(periods []segengine.ChargingPeriod) []chargingPeriodResponse {
	result := make([]chargingPeriodResponse, 0, len(periods))
	for _, period := range periods {
		dimensions := make([]dimensionResponse, 0, len(period.Dimensions))
		for _, dimension := range period.Dimensions {
			dimensions = append(dimensions, dimensionResponse{Type: string(dimension.Type), Volume: dimension.Volume})
		}
		result = append(result, chargingPeriodResponse{Start: period.Start.UTC().Format(time.RFC3339), Dimensions: dimensions})
	}
	return result
}

func toTraceEventsResponse(events []segengine.TraceEvent) []traceEventResponse {
	result := make([]traceEventResponse, 0, len(events))
	for _, event := range events {
		result = append(result, traceEventResponse{
			At:        event.At.UTC().Format(time.RFC3339),
			Reason:    string(event.Reason),
			Detail:    event.Detail,
			PeriodKey: event.PeriodKey,
		})
	}
	return result
}
