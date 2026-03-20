package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockPaymentsServer(t *testing.T, holdStatus int, holdResp HoldResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(holdStatus)
			json.NewEncoder(w).Encode(holdResp)
		case r.URL.Path == "/api/v1/payments/release" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/payments/transfer" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSaga_CreateContract_HoldSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusCreated, HoldResponse{
		ID:       holdID,
		WalletID: uuid.New().String(),
		Amount:   50000,
		Status:   "active",
	})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Build API",
		Amount:             50000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}

	result, err := saga.CreateContract(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, StatusHoldPending, result.Status)
	assert.Equal(t, &holdID, result.HoldID)
}

func TestSaga_HandlePaymentHeld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusHoldPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.HandlePaymentHeld(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusAwaitingAccept, got.Status)
}

func TestSaga_HandlePaymentFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusHoldPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.HandlePaymentFailed(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, got.Status)
}

func TestSaga_AcceptContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusAwaitingAccept,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.AcceptContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractAccepted, entries[0].EventType)
}

func TestSaga_CancelContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusAwaitingAccept,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CancelContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusDeclined, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractDeclined, entries[0].EventType)
}

func TestSaga_CompleteContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	freelancerWalletID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusActive,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: freelancerWalletID,
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CompleteContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractCompleted, entries[0].EventType)
}

func TestSaga_AcceptContract_WrongStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.AcceptContract(ctx, contractID)
	assert.ErrorIs(t, err, ErrStatusConflict)
}

func TestSaga_CompleteContract_RetryFromCompleting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusCompleting,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CompleteContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)
}
