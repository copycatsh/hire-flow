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

func setupTestMux(t *testing.T, jobsSrv, contractsSrv, paymentsSrv *httptest.Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if jobsSrv != nil {
		jh := &JobHandler{jobs: &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"}}
		jh.RegisterRoutes(mux)
	}
	if contractsSrv != nil {
		ch := &ContractHandler{contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"}}
		ch.RegisterRoutes(mux)
	}
	if paymentsSrv != nil {
		wh := &WalletHandler{payments: &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"}}
		wh.RegisterRoutes(mux)
	}
	return mux
}

func adminRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), bff.CtxKeyUserID, "44444444-4444-4444-4444-444444444444")
	ctx = context.WithValue(ctx, bff.CtxKeyRole, "admin")
	return req.WithContext(ctx)
}

func TestJobHandler_List_ForwardsAll(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/jobs", gotPath)
}

func TestJobHandler_List_WithQueryParams(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/jobs?status=open&limit=10", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "status=open&limit=10", gotRawQuery)
}

func TestContractHandler_List_NoFilter(t *testing.T) {
	var gotPath, gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/contracts", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/contracts", gotPath)
	assert.Empty(t, gotRawQuery, "admin contract list should not inject client_id or freelancer_id filters")
}

func TestWalletHandler_List(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, srv)
	req := adminRequest(t, http.MethodGet, "/api/v1/wallets", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/payments/wallets", gotPath)
}

func TestDashboard_Stats_AllServicesUp(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []string{}, "total": 12})
	}))
	defer jobsSrv.Close()

	contractsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []string{}, "total": 7})
	}))
	defer contractsSrv.Close()

	paymentsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []string{}, "total": 3})
	}))
	defer paymentsSrv.Close()

	dh := &DashboardHandler{
		jobs:      &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"},
		contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"},
		payments:  &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"},
	}

	mux := http.NewServeMux()
	dh.RegisterRoutes(mux)

	req := adminRequest(t, http.MethodGet, "/api/v1/dashboard/stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp dashboardResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 12, resp.Jobs.Total)
	assert.Equal(t, 7, resp.Contracts.Total)
	assert.Equal(t, 3, resp.Wallets.Total)
	assert.Empty(t, resp.Jobs.Error)
	assert.Empty(t, resp.Contracts.Error)
	assert.Empty(t, resp.Wallets.Error)
}

func TestDashboard_Stats_PartialFailure(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []string{}, "total": 5})
	}))
	defer jobsSrv.Close()

	// contracts and payments are down
	contractsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	contractsSrv.Close()

	paymentsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	paymentsSrv.Close()

	dh := &DashboardHandler{
		jobs:      &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"},
		contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"},
		payments:  &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"},
	}

	mux := http.NewServeMux()
	dh.RegisterRoutes(mux)

	req := adminRequest(t, http.MethodGet, "/api/v1/dashboard/stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp dashboardResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 5, resp.Jobs.Total)
	assert.Empty(t, resp.Jobs.Error)
	assert.Equal(t, 0, resp.Contracts.Total)
	assert.Contains(t, resp.Contracts.Error, "contracts")
	assert.Equal(t, 0, resp.Wallets.Total)
	assert.Contains(t, resp.Wallets.Error, "payments")
}

func TestServiceDown_Returns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
