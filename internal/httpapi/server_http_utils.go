package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxBodyBytes int64 = 1 << 20

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
