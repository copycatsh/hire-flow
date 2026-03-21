package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRouter(h *ContractHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestHandler_CreateContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusCreated, HoldResponse{
		ID:       holdID,
		WalletID: uuid.New().String(),
		Amount:   50000,
		Status:   "active",
	})
	defer ps.Close()

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, NewPaymentsClient(ps.URL))
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Build API",
		"description": "REST API development",
		"amount": 50000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `",
		"milestones": [
			{"title": "Phase 1", "description": "Design", "amount": 20000, "position": 0},
			{"title": "Phase 2", "description": "Implement", "amount": 30000, "position": 1}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, StatusHoldPending, resp.Status)
	assert.NotEmpty(t, resp.ID)
}

func TestHandler_CreateContract_ValidationError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	body := `{"title": "Missing fields"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_AcceptContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	contractID := uuid.New().String()
	c := Contract{
		ID: contractID, ClientID: uuid.New().String(), FreelancerID: uuid.New().String(),
		Title: "Test", Amount: 10000, Currency: "USD", Status: StatusAwaitingAccept,
		ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+contractID+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, StatusActive, resp.Status)
}

func TestHandler_AcceptContract_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+uuid.New().String()+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_ListContracts_ByClientID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	clientID := uuid.New().String()
	for i := range 3 {
		c := Contract{
			ID: uuid.New().String(), ClientID: clientID, FreelancerID: uuid.New().String(),
			Title: "Contract " + strings.Repeat("x", i), Amount: 10000, Currency: "USD", Status: StatusPending,
			ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
		}
		require.NoError(t, contractStore.Create(ctx, db, c))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts?client_id="+clientID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp []Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp, 3)
	for _, c := range resp {
		assert.Equal(t, clientID, c.ClientID)
	}
}

func TestHandler_ListContracts_ByFreelancerID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	freelancerID := uuid.New().String()
	c := Contract{
		ID: uuid.New().String(), ClientID: uuid.New().String(), FreelancerID: freelancerID,
		Title: "Freelancer Contract", Amount: 20000, Currency: "USD", Status: StatusActive,
		ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts?freelancer_id="+freelancerID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp []Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp, 1)
	assert.Equal(t, freelancerID, resp[0].FreelancerID)
}

func TestHandler_ListContracts_EmptyResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts?client_id="+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp []Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp, 0)
}

func TestHandler_ListContracts_MissingFilter(t *testing.T) {
	h := &ContractHandler{}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "client_id or freelancer_id required", resp["error"])
}

func TestHandler_AcceptContract_WrongStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	contractID := uuid.New().String()
	c := Contract{
		ID: contractID, ClientID: uuid.New().String(), FreelancerID: uuid.New().String(),
		Title: "Test", Amount: 10000, Currency: "USD", Status: StatusPending,
		ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+contractID+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}
