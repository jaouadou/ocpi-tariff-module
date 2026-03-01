package httpapi

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateSessionInvalidTimezone(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	requestBody := `{
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Nowhere",
		"tariff":{
			"elements":[
				{
					"id":"base",
					"price_components":[{"type":"ENERGY"}],
					"restrictions":{}
				}
			]
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(requestBody))
	rec := httptest.NewRecorder()

	server.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Error.Code != "invalid_timezone" {
		t.Fatalf("expected error code invalid_timezone, got %q", response.Error.Code)
	}
}

func TestCreateSessionDuplicateSessionID(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	requestBody := `{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{
			"elements":[
				{
					"id":"base",
					"price_components":[{"type":"ENERGY"}],
					"restrictions":{}
				}
			]
		}
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(requestBody))
	firstRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(firstRec, firstReq)

	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first request status %d, got %d", http.StatusCreated, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(requestBody))
	secondRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status %d, got %d", http.StatusConflict, secondRec.Code)
	}

	var response errorResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Error.Code != "duplicate_session_id" {
		t.Fatalf("expected error code duplicate_session_id, got %q", response.Error.Code)
	}
}

func TestAppendMeterSamplesIdempotent(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	createSessionRequestBody := `{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{
			"elements":[
				{
					"id":"base",
					"price_components":[{"type":"ENERGY"}],
					"restrictions":{}
				}
			]
		}
	}`

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(createSessionRequestBody))
	createRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	ingestRequestBody := `{
		"samples":[
			{"id":"m-1","at":"2026-02-22T10:01:00Z","total_kwh":123.45}
		]
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/meter-samples", bytes.NewBufferString(ingestRequestBody))
	firstRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(firstRec, firstReq)

	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected first ingest status %d, got %d", http.StatusAccepted, firstRec.Code)
	}

	var firstResponse ingestSamplesResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("decode first ingest response: %v", err)
	}

	if firstResponse.Accepted != 1 || firstResponse.Duplicates != 0 {
		t.Fatalf("expected first response accepted=1 duplicates=0, got accepted=%d duplicates=%d", firstResponse.Accepted, firstResponse.Duplicates)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/meter-samples", bytes.NewBufferString(ingestRequestBody))
	secondRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected second ingest status %d, got %d", http.StatusAccepted, secondRec.Code)
	}

	var secondResponse ingestSamplesResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("decode second ingest response: %v", err)
	}

	if secondResponse.Accepted != 0 || secondResponse.Duplicates != 1 {
		t.Fatalf("expected second response accepted=0 duplicates=1, got accepted=%d duplicates=%d", secondResponse.Accepted, secondResponse.Duplicates)
	}
}

func TestQueryPeriodsSessionNotFound(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/33333333-3333-4333-8333-333333333333/periods", nil)
	rec := httptest.NewRecorder()

	server.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var response errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Error.Code != "session_not_found" {
		t.Fatalf("expected error code session_not_found, got %q", response.Error.Code)
	}
}

func TestQueryPeriodsNormalizesMeterAndIncludesTrace(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())

	createSessionBody := `{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{
			"elements":[
				{
					"id":"base",
					"price_components":[{"type":"ENERGY"},{"type":"TIME"}],
					"restrictions":{}
				}
			]
		}
	}`

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(createSessionBody))
	createRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	ingestBody := `{
		"samples":[
			{"id":"m-1","at":"2026-02-22T10:00:00Z","total_kwh":100.0},
			{"id":"m-2","at":"2026-02-22T11:00:00Z","total_kwh":101.0}
		]
	}`

	ingestReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/meter-samples", bytes.NewBufferString(ingestBody))
	ingestRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusAccepted {
		t.Fatalf("expected ingest status %d, got %d", http.StatusAccepted, ingestRec.Code)
	}

	periodsReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/11111111-1111-4111-8111-111111111111/periods?trace=1", nil)
	periodsRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(periodsRec, periodsReq)

	if periodsRec.Code != http.StatusOK {
		t.Fatalf("expected periods status %d, got %d", http.StatusOK, periodsRec.Code)
	}

	var response periodsResponse
	if err := json.Unmarshal(periodsRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode periods response: %v", err)
	}

	if response.EndUTC != "2026-02-22T11:00:00Z" {
		t.Fatalf("expected end_utc to default to latest meter sample, got %q", response.EndUTC)
	}

	if len(response.Periods) == 0 {
		t.Fatal("expected at least one period")
	}

	energy := -1.0
	for _, dimension := range response.Periods[0].Dimensions {
		if dimension.Type == "ENERGY" {
			energy = dimension.Volume
			break
		}
	}

	if energy < 0 {
		t.Fatal("expected ENERGY dimension in first period")
	}
	if math.Abs(energy-1.0) > 1e-9 {
		t.Fatalf("expected normalized energy to be 1.0 kWh, got %.12f", energy)
	}

	if response.Trace == nil || len(response.Trace.Events) == 0 {
		t.Fatal("expected trace.events to be present when trace=1")
	}
}

func TestEndSessionNotFound(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/33333333-3333-4333-8333-333333333333/end", bytes.NewBufferString(`{"end_utc":"2026-02-22T11:00:00Z"}`))
	rec := httptest.NewRecorder()

	server.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestEndSessionInvalidEndUTC(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{"elements":[{"id":"base","price_components":[{"type":"TIME"}],"restrictions":{}}]}
	}`))
	createRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/end", bytes.NewBufferString(`{"end_utc":"not-a-time"}`))
	rec := httptest.NewRecorder()

	server.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestEndSessionAlreadyEnded(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{"elements":[{"id":"base","price_components":[{"type":"ENERGY"}],"restrictions":{}}]}
	}`))
	createRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	ingestReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/meter-samples", bytes.NewBufferString(`{
		"samples":[
			{"id":"m-1","at":"2026-02-22T10:00:00Z","total_kwh":100.0},
			{"id":"m-2","at":"2026-02-22T11:00:00Z","total_kwh":101.0}
		]
	}`))
	ingestRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusAccepted {
		t.Fatalf("expected ingest status %d, got %d", http.StatusAccepted, ingestRec.Code)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/end", bytes.NewBufferString(`{"end_utc":"2026-02-22T11:00:00Z"}`))
	firstRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first end status %d, got %d", http.StatusOK, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/end", bytes.NewBufferString(`{"end_utc":"2026-02-22T11:01:00Z"}`))
	secondRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected second end status %d, got %d", http.StatusConflict, secondRec.Code)
	}
}

func TestGetCDRStatesAndSealedResponse(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{
		"session_id":"11111111-1111-4111-8111-111111111111",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{
			"elements":[
				{"id":"base","price_components":[{"type":"ENERGY"},{"type":"TIME"}],"restrictions":{}}
			]
		}
	}`))
	createRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	notEndedReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/11111111-1111-4111-8111-111111111111/cdr", nil)
	notEndedRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(notEndedRec, notEndedReq)
	if notEndedRec.Code != http.StatusConflict {
		t.Fatalf("expected status %d before end, got %d", http.StatusConflict, notEndedRec.Code)
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/33333333-3333-4333-8333-333333333333/cdr", nil)
	notFoundRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d for unknown session, got %d", http.StatusNotFound, notFoundRec.Code)
	}

	ingestReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/meter-samples", bytes.NewBufferString(`{
		"samples":[
			{"id":"m-1","at":"2026-02-22T10:00:00Z","total_kwh":100.0},
			{"id":"m-2","at":"2026-02-22T11:00:00Z","total_kwh":101.0}
		]
	}`))
	ingestRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusAccepted {
		t.Fatalf("expected ingest status %d, got %d", http.StatusAccepted, ingestRec.Code)
	}

	endReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/11111111-1111-4111-8111-111111111111/end", bytes.NewBufferString(`{"end_utc":"2026-02-22T11:00:00Z"}`))
	endRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(endRec, endReq)
	if endRec.Code != http.StatusOK {
		t.Fatalf("expected end status %d, got %d", http.StatusOK, endRec.Code)
	}

	firstCDRReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/11111111-1111-4111-8111-111111111111/cdr", nil)
	firstCDRRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(firstCDRRec, firstCDRReq)
	if firstCDRRec.Code != http.StatusOK {
		t.Fatalf("expected cdr status %d, got %d", http.StatusOK, firstCDRRec.Code)
	}

	var firstCDR cdrResponse
	if err := json.Unmarshal(firstCDRRec.Body.Bytes(), &firstCDR); err != nil {
		t.Fatalf("decode first cdr response: %v", err)
	}

	if firstCDR.SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("expected session_id 11111111-1111-4111-8111-111111111111, got %q", firstCDR.SessionID)
	}
	if firstCDR.StartUTC != "2026-02-22T10:00:00Z" {
		t.Fatalf("expected start_utc to match session start, got %q", firstCDR.StartUTC)
	}
	if firstCDR.EndUTC != "2026-02-22T11:00:00Z" {
		t.Fatalf("expected end_utc to match end payload, got %q", firstCDR.EndUTC)
	}
	if _, err := time.Parse(time.RFC3339, firstCDR.FinalizedAt); err != nil {
		t.Fatalf("expected finalized_at to be RFC3339, got %q", firstCDR.FinalizedAt)
	}
	if len(firstCDR.ChargingPeriods) == 0 {
		t.Fatal("expected charging_periods to be present")
	}

	secondCDRReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/11111111-1111-4111-8111-111111111111/cdr", nil)
	secondCDRRec := httptest.NewRecorder()
	server.Mux().ServeHTTP(secondCDRRec, secondCDRReq)
	if secondCDRRec.Code != http.StatusOK {
		t.Fatalf("expected second cdr status %d, got %d", http.StatusOK, secondCDRRec.Code)
	}

	var secondCDR cdrResponse
	if err := json.Unmarshal(secondCDRRec.Body.Bytes(), &secondCDR); err != nil {
		t.Fatalf("decode second cdr response: %v", err)
	}

	if secondCDR.FinalizedAt != firstCDR.FinalizedAt {
		t.Fatalf("expected finalized_at to be stable, first=%q second=%q", firstCDR.FinalizedAt, secondCDR.FinalizedAt)
	}
}

func TestHTTPFlow_Create_Ingest_Periods_End_CDR(t *testing.T) {
	t.Parallel()

	server := NewServer(NewSessionStore())
	mux := server.Mux()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{
		"session_id":"22222222-2222-4222-8222-222222222222",
		"start_utc":"2026-02-22T10:00:00Z",
		"timezone":"Europe/Paris",
		"tariff":{
			"elements":[
				{"id":"base","price_components":[{"type":"ENERGY"},{"type":"TIME"}],"restrictions":{}}
			]
		}
	}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	var createResp createSessionResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("expected session_id 22222222-2222-4222-8222-222222222222, got %q", createResp.SessionID)
	}
	if createResp.StartUTC != "2026-02-22T10:00:00Z" {
		t.Fatalf("expected start_utc 2026-02-22T10:00:00Z, got %q", createResp.StartUTC)
	}
	if createResp.Timezone != "Europe/Paris" {
		t.Fatalf("expected timezone Europe/Paris, got %q", createResp.Timezone)
	}

	ingestReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/22222222-2222-4222-8222-222222222222/meter-samples", bytes.NewBufferString(`{
		"samples":[
			{"id":"m-1","at":"2026-02-22T10:00:00Z","total_kwh":100.0},
			{"id":"m-2","at":"2026-02-22T10:30:00Z","total_kwh":100.5}
		]
	}`))
	ingestRec := httptest.NewRecorder()
	mux.ServeHTTP(ingestRec, ingestReq)

	if ingestRec.Code != http.StatusAccepted {
		t.Fatalf("expected ingest status %d, got %d", http.StatusAccepted, ingestRec.Code)
	}

	var ingestResp ingestSamplesResponse
	if err := json.Unmarshal(ingestRec.Body.Bytes(), &ingestResp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	if ingestResp.Accepted != 2 || ingestResp.Duplicates != 0 {
		t.Fatalf("expected first ingest accepted=2 duplicates=0, got accepted=%d duplicates=%d", ingestResp.Accepted, ingestResp.Duplicates)
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/22222222-2222-4222-8222-222222222222/meter-samples", bytes.NewBufferString(`{
		"samples":[
			{"id":"m-2","at":"2026-02-22T10:30:00Z","total_kwh":100.5}
		]
	}`))
	duplicateRec := httptest.NewRecorder()
	mux.ServeHTTP(duplicateRec, duplicateReq)

	if duplicateRec.Code != http.StatusAccepted {
		t.Fatalf("expected duplicate ingest status %d, got %d", http.StatusAccepted, duplicateRec.Code)
	}

	var duplicateResp ingestSamplesResponse
	if err := json.Unmarshal(duplicateRec.Body.Bytes(), &duplicateResp); err != nil {
		t.Fatalf("decode duplicate ingest response: %v", err)
	}
	if duplicateResp.Accepted != 0 || duplicateResp.Duplicates != 1 {
		t.Fatalf("expected duplicate ingest accepted=0 duplicates=1, got accepted=%d duplicates=%d", duplicateResp.Accepted, duplicateResp.Duplicates)
	}

	periodsReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/22222222-2222-4222-8222-222222222222/periods?as_of_utc=2026-02-22T10:30:00Z&trace=1", nil)
	periodsRec := httptest.NewRecorder()
	mux.ServeHTTP(periodsRec, periodsReq)

	if periodsRec.Code != http.StatusOK {
		t.Fatalf("expected periods status %d, got %d", http.StatusOK, periodsRec.Code)
	}

	var periodsResp periodsResponse
	if err := json.Unmarshal(periodsRec.Body.Bytes(), &periodsResp); err != nil {
		t.Fatalf("decode periods response: %v", err)
	}
	if periodsResp.SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("expected periods session_id 22222222-2222-4222-8222-222222222222, got %q", periodsResp.SessionID)
	}
	if periodsResp.StartUTC != "2026-02-22T10:00:00Z" {
		t.Fatalf("expected periods start_utc 2026-02-22T10:00:00Z, got %q", periodsResp.StartUTC)
	}
	if periodsResp.EndUTC != "2026-02-22T10:30:00Z" {
		t.Fatalf("expected periods end_utc 2026-02-22T10:30:00Z, got %q", periodsResp.EndUTC)
	}
	if len(periodsResp.Periods) == 0 {
		t.Fatal("expected periods to be non-empty")
	}
	if periodsResp.Trace == nil || len(periodsResp.Trace.Events) == 0 {
		t.Fatal("expected trace.events to be present when trace=1")
	}

	endReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/22222222-2222-4222-8222-222222222222/end", bytes.NewBufferString(`{"end_utc":"2026-02-22T10:30:00Z"}`))
	endRec := httptest.NewRecorder()
	mux.ServeHTTP(endRec, endReq)

	if endRec.Code != http.StatusOK {
		t.Fatalf("expected end status %d, got %d", http.StatusOK, endRec.Code)
	}

	var endResp endSessionResponse
	if err := json.Unmarshal(endRec.Body.Bytes(), &endResp); err != nil {
		t.Fatalf("decode end response: %v", err)
	}
	if endResp.SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("expected end session_id 22222222-2222-4222-8222-222222222222, got %q", endResp.SessionID)
	}
	if endResp.EndUTC != "2026-02-22T10:30:00Z" {
		t.Fatalf("expected end end_utc 2026-02-22T10:30:00Z, got %q", endResp.EndUTC)
	}

	cdrReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/22222222-2222-4222-8222-222222222222/cdr", nil)
	cdrRec := httptest.NewRecorder()
	mux.ServeHTTP(cdrRec, cdrReq)

	if cdrRec.Code != http.StatusOK {
		t.Fatalf("expected cdr status %d, got %d", http.StatusOK, cdrRec.Code)
	}

	var cdrResp cdrResponse
	if err := json.Unmarshal(cdrRec.Body.Bytes(), &cdrResp); err != nil {
		t.Fatalf("decode cdr response: %v", err)
	}
	if cdrResp.SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("expected cdr session_id 22222222-2222-4222-8222-222222222222, got %q", cdrResp.SessionID)
	}
	if cdrResp.StartUTC != "2026-02-22T10:00:00Z" {
		t.Fatalf("expected cdr start_utc 2026-02-22T10:00:00Z, got %q", cdrResp.StartUTC)
	}
	if cdrResp.EndUTC != "2026-02-22T10:30:00Z" {
		t.Fatalf("expected cdr end_utc 2026-02-22T10:30:00Z, got %q", cdrResp.EndUTC)
	}
	if _, err := time.Parse(time.RFC3339, cdrResp.FinalizedAt); err != nil {
		t.Fatalf("expected finalized_at RFC3339, got %q", cdrResp.FinalizedAt)
	}
	if len(cdrResp.ChargingPeriods) == 0 {
		t.Fatal("expected charging_periods to be non-empty")
	}
}
