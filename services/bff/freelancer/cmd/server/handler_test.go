package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/copycatsh/hire-flow/pkg/bff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMux(t *testing.T, matchSrv, contractsSrv, paymentsSrv *httptest.Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

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
	ctx = context.WithValue(ctx, bff.CtxKeyRole, "freelancer")
	return req.WithContext(ctx)
}

func TestMatchHandler_FindJobMatches(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"matches": []string{}})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/matches?profile_id=prof-42", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/match/profile/prof-42", gotPath)
}

func TestMatchHandler_MissingProfileID(t *testing.T) {
	mux := setupTestMux(t, nil, nil, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	mux2 := setupTestMux(t, srv, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/matches", nil)
	w := httptest.NewRecorder()

	_ = mux
	mux2.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestContractHandler_List(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/contracts", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/contracts", gotPath)
	assert.Equal(t, "freelancer_id=user-1", gotQuery)
}

func TestContractHandler_GetByID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "c-123"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/contracts/c-123", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/contracts/c-123", gotPath)
}

func TestContractHandler_Accept(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil)
	req := authedRequest(t, http.MethodPut, "/api/v1/contracts/c-123/accept", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/contracts/c-123/accept", gotPath)
	assert.Equal(t, http.MethodPut, gotMethod)
}

func TestPaymentHandler_GetBalance(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"balance": 100.0})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, srv)
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

	mux := setupTestMux(t, nil, srv, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/contracts", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-1", gotUserID)
}
