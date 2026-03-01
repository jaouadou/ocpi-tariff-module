package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

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

	sessionIDStr := strings.TrimSpace(req.SessionID)
	var sessionID uuid.UUID

	if sessionIDStr == "" {
		sessionID, err = uuid.NewRandom()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate session_id")
			return
		}
	} else {
		sessionID, err = uuid.Parse(sessionIDStr)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be a valid UUID")
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
		SessionID: sessionID.String(),
		StartUTC:  state.StartUTC.Format(time.RFC3339),
		Timezone:  req.Timezone,
	})
}

func (s *Server) endSessionHandler(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	sessionID, err := uuid.Parse(pathID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_session_id", err.Error())
		return
	}

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

	snapshot, err := s.store.EndSession(sessionID, endUTC.UTC())
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
	sessionID, err := uuid.Parse(pathID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_session_id", err.Error())
		return
	}

	snapshot, err := s.store.SnapshotSession(sessionID)
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
