package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

const maxBodyBytes int64 = 1 << 20

type Server struct {
	store     *SessionStore
	finalizer *segengine.Finalizer
}

func NewServer(store *SessionStore) *Server {
	return &Server{store: store, finalizer: segengine.NewFinalizer()}
}

func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthzHandler)
	mux.HandleFunc("GET /version", s.versionHandler)
	mux.HandleFunc("POST /v1/sessions", s.createSessionHandler)
	mux.HandleFunc("POST /v1/sessions/{id}/meter-samples", s.appendMeterSamplesHandler)
	mux.HandleFunc("POST /v1/sessions/{id}/power-samples", s.appendPowerSamplesHandler)
	mux.HandleFunc("POST /v1/sessions/{id}/current-samples", s.appendCurrentSamplesHandler)
	mux.HandleFunc("GET /v1/sessions/{id}/periods", s.queryPeriodsHandler)
	mux.HandleFunc("POST /v1/sessions/{id}/end", s.endSessionHandler)
	mux.HandleFunc("GET /v1/sessions/{id}/cdr", s.getCDRHandler)
	return mux
}

func (s *Server) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) versionHandler(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"module":  "github.com/jaouadou/ocpi-tariff-module",
		"version": "dev",
	})
}

func (s *Server) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	startUTC, err := time.Parse(time.RFC3339, req.StartUTC)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_start_utc", "start_utc must be RFC3339")
		return
	}

	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_timezone", "timezone must be a valid IANA timezone")
		return
	}

	tariff, err := req.Tariff.toSegengineTariff()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_tariff", err.Error())
		return
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID, err = generateUUIDv4()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate session_id")
			return
		}
	}

	state := &SessionState{
		ID:             sessionID,
		StartUTC:       startUTC.UTC(),
		Timezone:       req.Timezone,
		Location:       loc,
		Tariff:         tariff,
		MeterSamples:   make(map[string]segengine.MeterSample),
		PowerSamples:   make(map[string]segengine.PowerSample),
		CurrentSamples: make(map[string]segengine.CurrentSample),
	}

	if err := s.store.CreateSession(state); err != nil {
		if errors.Is(err, ErrSessionExists) {
			s.writeError(w, http.StatusConflict, "duplicate_session_id", "session_id already exists")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to persist session")
		return
	}

	s.writeJSON(w, http.StatusCreated, createSessionResponse{
		SessionID: sessionID,
		StartUTC:  state.StartUTC.Format(time.RFC3339),
		Timezone:  req.Timezone,
	})
}

func (s *Server) appendMeterSamplesHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	var req meterSamplesRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	samples := make([]identifiedMeterSample, 0, len(req.Samples))
	for i, rawSample := range req.Samples {
		sampleID := strings.TrimSpace(rawSample.ID)
		if sampleID == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_id", fmt.Sprintf("samples[%d].id must be non-empty", i))
			return
		}

		at, err := time.Parse(time.RFC3339, rawSample.At)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_timestamp", fmt.Sprintf("samples[%d].at must be RFC3339", i))
			return
		}

		samples = append(samples, identifiedMeterSample{
			id: sampleID,
			sample: segengine.MeterSample{
				At:       at.UTC(),
				TotalKWh: rawSample.TotalKWh,
			},
		})
	}

	accepted, duplicates, err := s.store.AppendMeterSamples(pathID, samples)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to append meter samples")
		return
	}

	s.writeJSON(w, http.StatusAccepted, ingestSamplesResponse{Accepted: accepted, Duplicates: duplicates})
}

func (s *Server) appendPowerSamplesHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	var req powerSamplesRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	samples := make([]identifiedPowerSample, 0, len(req.Samples))
	for i, rawSample := range req.Samples {
		sampleID := strings.TrimSpace(rawSample.ID)
		if sampleID == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_id", fmt.Sprintf("samples[%d].id must be non-empty", i))
			return
		}

		at, err := time.Parse(time.RFC3339, rawSample.At)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_timestamp", fmt.Sprintf("samples[%d].at must be RFC3339", i))
			return
		}

		samples = append(samples, identifiedPowerSample{
			id: sampleID,
			sample: segengine.PowerSample{
				At:      at.UTC(),
				PowerKW: rawSample.PowerKW,
			},
		})
	}

	accepted, duplicates, err := s.store.AppendPowerSamples(pathID, samples)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to append power samples")
		return
	}

	s.writeJSON(w, http.StatusAccepted, ingestSamplesResponse{Accepted: accepted, Duplicates: duplicates})
}

func (s *Server) appendCurrentSamplesHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	var req currentSamplesRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	samples := make([]identifiedCurrentSample, 0, len(req.Samples))
	for i, rawSample := range req.Samples {
		sampleID := strings.TrimSpace(rawSample.ID)
		if sampleID == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_id", fmt.Sprintf("samples[%d].id must be non-empty", i))
			return
		}

		at, err := time.Parse(time.RFC3339, rawSample.At)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_timestamp", fmt.Sprintf("samples[%d].at must be RFC3339", i))
			return
		}

		samples = append(samples, identifiedCurrentSample{
			id: sampleID,
			sample: segengine.CurrentSample{
				At:       at.UTC(),
				CurrentA: rawSample.CurrentA,
			},
		})
	}

	accepted, duplicates, err := s.store.AppendCurrentSamples(pathID, samples)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to append current samples")
		return
	}

	s.writeJSON(w, http.StatusAccepted, ingestSamplesResponse{Accepted: accepted, Duplicates: duplicates})
}

func (s *Server) queryPeriodsHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	snapshot, err := s.store.SnapshotSession(pathID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
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
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "periods_unavailable", err.Error())
			return
		}

		periodsResponsePayload.Periods = toPeriodsResponse(periods)
		periodsResponsePayload.Trace = &traceResponse{Events: toTraceEventsResponse(trace.Events)}
		s.writeJSON(w, http.StatusOK, periodsResponsePayload)
		return
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
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "periods_unavailable", err.Error())
		return
	}

	periodsResponsePayload.Periods = toPeriodsResponse(periods)
	s.writeJSON(w, http.StatusOK, periodsResponsePayload)
}

func (s *Server) endSessionHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	var req endSessionRequest
	if err := s.readJSON(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	endUTC, err := time.Parse(time.RFC3339, req.EndUTC)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_end_utc", "end_utc must be RFC3339")
		return
	}

	snapshot, err := s.store.EndSession(pathID, endUTC.UTC())
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		if errors.Is(err, ErrSessionAlreadyEnded) {
			s.writeError(w, http.StatusConflict, "session_already_ended", "session already ended")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to end session")
		return
	}

	if _, err := s.finalizeSessionSnapshot(snapshot); err != nil {
		s.writeError(w, http.StatusBadRequest, "periods_unavailable", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, endSessionResponse{SessionID: snapshot.ID, EndUTC: snapshot.EndUTC.UTC().Format(time.RFC3339)})
}

func (s *Server) getCDRHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")

	snapshot, err := s.store.SnapshotSession(pathID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to load session")
		return
	}

	if snapshot.EndUTC == nil {
		s.writeError(w, http.StatusConflict, "session_not_ended", "session not ended")
		return
	}

	sealed, err := s.finalizeSessionSnapshot(snapshot)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "periods_unavailable", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, cdrResponse{
		SessionID:       sealed.SessionID,
		StartUTC:        sealed.Start.UTC().Format(time.RFC3339),
		EndUTC:          sealed.End.UTC().Format(time.RFC3339),
		FinalizedAt:     sealed.FinalizedAt.UTC().Format(time.RFC3339),
		ChargingPeriods: toPeriodsResponse(sealed.ChargingPeriods),
	})
}

func (s *Server) finalizeSessionSnapshot(snapshot SessionSnapshot) (segengine.CDR, error) {
	if snapshot.EndUTC == nil {
		return segengine.CDR{}, errors.New("session not ended")
	}

	periods, err := computePeriods(snapshot, snapshot.EndUTC.UTC(), false)
	if err != nil {
		return segengine.CDR{}, err
	}

	sealed, ok := s.finalizer.TryFinalize(
		snapshot.ID,
		snapshot.StartUTC,
		snapshot.EndUTC.UTC(),
		snapshot.EndUTC.UTC(),
		periods,
		time.Now().UTC(),
	)
	if !ok {
		return segengine.CDR{}, errors.New("failed to finalize cdr")
	}

	return sealed, nil
}

func computePeriods(snapshot SessionSnapshot, effectiveEndUTC time.Time, includeTrace bool) ([]segengine.ChargingPeriod, error) {
	if !snapshot.StartUTC.Before(effectiveEndUTC) {
		return []segengine.ChargingPeriod{}, nil
	}

	normalizedMeter := normalizeMeterSamples(snapshot.MeterSamples)
	calendarBoundaries := collectCalendarBoundaries(snapshot.StartUTC, effectiveEndUTC, snapshot.Location, snapshot.Tariff)
	thresholds := collectEnergyThresholds(snapshot.Tariff)

	if includeTrace {
		trace := &segengine.Trace{}
		return segengine.AccumulateWithTrace(
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
	}

	return segengine.Accumulate(
		snapshot.StartUTC,
		effectiveEndUTC,
		snapshot.Tariff,
		normalizedMeter,
		snapshot.PowerSamples,
		snapshot.CurrentSamples,
		calendarBoundaries,
		thresholds,
	)
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
		calendar := segengine.TariffRestrictionsCalendar{
			StartTime:  element.Restrictions.StartTime,
			EndTime:    element.Restrictions.EndTime,
			StartDate:  element.Restrictions.StartDate,
			EndDate:    element.Restrictions.EndDate,
			DaysOfWeek: element.Restrictions.DayOfWeek,
		}
		for _, boundary := range segengine.CalendarBoundaries(startUTC, endUTC, location, calendar) {
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

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

func generateUUIDv4() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	buf := make([]byte, 36)
	hex.Encode(buf[0:8], raw[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], raw[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], raw[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], raw[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], raw[10:16])

	return string(buf), nil
}
