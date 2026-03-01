package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

type appendSamplesHandlerOptions[RawSample identifiedRawSample, TargetSample any] struct {
	newRequest        func() any
	extractSamples    func(any) []RawSample
	toTarget          func(RawSample, time.Time) TargetSample
	appendToStore     func(uuid.UUID, []identifiedSample[TargetSample]) (accepted, duplicates int, err error)
	appendFailureText string
}

func (s *Server) appendMeterSamplesHandler(w http.ResponseWriter, r *http.Request) {
	options := appendSamplesHandlerOptions[meterSampleRequest, segengine.MeterSample]{
		newRequest: func() any {
			return &meterSamplesRequest{}
		},
		extractSamples: func(req any) []meterSampleRequest {
			return req.(*meterSamplesRequest).Samples
		},
		toTarget: func(sample meterSampleRequest, at time.Time) segengine.MeterSample {
			return segengine.MeterSample{At: at.UTC(), TotalKWh: sample.TotalKWh}
		},
		appendToStore:     s.store.AppendMeterSamples,
		appendFailureText: "failed to append meter samples",
	}

	handleAppendSamples(s, w, r, options)
}

func (s *Server) appendPowerSamplesHandler(w http.ResponseWriter, r *http.Request) {
	options := appendSamplesHandlerOptions[powerSampleRequest, segengine.PowerSample]{
		newRequest: func() any {
			return &powerSamplesRequest{}
		},
		extractSamples: func(req any) []powerSampleRequest {
			return req.(*powerSamplesRequest).Samples
		},
		toTarget: func(sample powerSampleRequest, at time.Time) segengine.PowerSample {
			return segengine.PowerSample{At: at.UTC(), PowerKW: sample.PowerKW}
		},
		appendToStore:     s.store.AppendPowerSamples,
		appendFailureText: "failed to append power samples",
	}

	handleAppendSamples(s, w, r, options)
}

func (s *Server) appendCurrentSamplesHandler(w http.ResponseWriter, r *http.Request) {
	options := appendSamplesHandlerOptions[currentSampleRequest, segengine.CurrentSample]{
		newRequest: func() any {
			return &currentSamplesRequest{}
		},
		extractSamples: func(req any) []currentSampleRequest {
			return req.(*currentSamplesRequest).Samples
		},
		toTarget: func(sample currentSampleRequest, at time.Time) segengine.CurrentSample {
			return segengine.CurrentSample{At: at.UTC(), CurrentA: sample.CurrentA}
		},
		appendToStore:     s.store.AppendCurrentSamples,
		appendFailureText: "failed to append current samples",
	}

	handleAppendSamples(s, w, r, options)
}

type identifiedRawSample interface {
	sampleID() string
	sampleAt() string
}

func handleAppendSamples[RawSample identifiedRawSample, TargetSample any](s *Server, w http.ResponseWriter, r *http.Request, options appendSamplesHandlerOptions[RawSample, TargetSample]) {
	pathID := r.PathValue("id")
	sessionID, err := uuid.Parse(pathID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_session_id", err.Error())
		return
	}

	req := options.newRequest()
	if err := s.readJSON(w, r, req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	rawSamples := options.extractSamples(req)

	samples := make([]identifiedSample[TargetSample], 0, len(rawSamples))
	for i, rawSample := range rawSamples {
		id := strings.TrimSpace(rawSample.sampleID())
		if id == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_id", fmt.Sprintf("samples[%d].id must be non-empty", i))
			return
		}

		parsedAt, err := time.Parse(time.RFC3339, rawSample.sampleAt())
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_sample_timestamp", fmt.Sprintf("samples[%d].at must be RFC3339", i))
			return
		}

		samples = append(samples, identifiedSample[TargetSample]{
			id:     id,
			sample: options.toTarget(rawSample, parsedAt),
		})
	}

	accepted, duplicates, err := options.appendToStore(sessionID, samples)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "session_not_found", "session not found")
			return
		}

		s.writeError(w, http.StatusInternalServerError, "internal_error", options.appendFailureText)
		return
	}

	s.writeJSON(w, http.StatusAccepted, ingestSamplesResponse{Accepted: accepted, Duplicates: duplicates})
}
