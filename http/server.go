package httpapi

import (
	"net/http"

	segengine "github.com/jaouadou/ocpi-tariff-module/pkg/segengine"
)

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
