package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/copycatsh/hire-flow/pkg/bff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMux(t *testing.T, jobsSrv, matchSrv, contractsSrv, paymentsSrv *httptest.Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

	if jobsSrv != nil {
		jh := &JobHandler{jobs: &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"}}
		jh.RegisterRoutes(mux)
	}
	if matchSrv != nil {
		mh := &MatchHandler{matching: &bff.ServiceClient{BaseURL: matchSrv.URL, HTTP: matchSrv.Client(), Name: "matching"}}
		mh.RegisterRoutes(mux)
	}
	if contractsSrv != nil {
		ch := &ContractHandler{contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"}}
		ch.RegisterRoutes(mux)
	}
	if paymentsSrv != nil {
		ph := &PaymentHandler{payments: &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"}}
		ph.RegisterRoutes(mux)
	}

	return mux
}

func authedRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), bff.CtxKeyUserID, "user-1")
	ctx = context.WithValue(ctx, bff.CtxKeyRole, "client")
	return req.WithContext(ctx)
}

func TestJobHandler_Create(t *testing.T) {
	var gotPath, gotMethod, gotUserID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotUserID = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "job-1"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/jobs", strings.NewReader(`{"title":"Go Dev"}`))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "/api/v1/jobs", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "user-1", gotUserID)
}

func TestJobHandler_List_WithQueryParams(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs?status=open&limit=10", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "status=open&limit=10", gotRawQuery)
}

func TestJobHandler_GetByID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "abc-123"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs/abc-123", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/jobs/abc-123", gotPath)
}

func TestJobHandler_ServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	mux := setupTestMux(t, srv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestMatchHandler_FindMatches(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"matches": []string{}})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/jobs/job-42/matches", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/match/job/job-42", gotPath)
}

func TestContractHandler_Create(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "contract-1"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, srv, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/contracts", strings.NewReader(`{"job_id":"j1"}`))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, http.MethodPost, gotMethod)
}

func TestContractHandler_GetByID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, srv, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/contracts/nonexistent", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPaymentHandler_GetBalance(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"balance": 100.0})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, nil, srv)
	req := authedRequest(t, http.MethodGet, "/api/v1/wallet", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/payments/wallet/user-1", gotPath)
}

func TestXUserIDForwarding(t *testing.T) {
	var gotUserID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil, nil)

	req := authedRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-1", gotUserID)
}
