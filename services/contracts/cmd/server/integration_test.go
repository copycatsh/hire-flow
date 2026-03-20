package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

func setupNATS(t *testing.T, ctx context.Context) *NATSClient {
	t.Helper()

	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := NewNATSClient(natsURL)
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	require.NoError(t, nc.EnsureStream(ctx))
	return nc
}

func TestIntegration_FullHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	nc := setupNATS(t, ctx)

	holdID := uuid.New().String()
	var transferCalled atomic.Bool
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(HoldResponse{
				ID: holdID, WalletID: uuid.New().String(), Amount: 50000, Status: "active",
			})
		case r.URL.Path == "/api/v1/payments/transfer":
			transferCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. Create contract
	clientWalletID := uuid.New().String()
	freelancerWalletID := uuid.New().String()
	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Full Flow Test",
		"description": "E2E test",
		"amount": 50000,
		"client_wallet_id": "` + clientWalletID + `",
		"freelancer_wallet_id": "` + freelancerWalletID + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, StatusHoldPending, created.Status)

	// 2. Simulate payment.held event → AWAITING_ACCEPT
	require.NoError(t, saga.HandlePaymentHeld(ctx, created.ID))
	got, _ := contractStore.GetByID(ctx, db, created.ID)
	assert.Equal(t, StatusAwaitingAccept, got.Status)

	// 3. Accept → ACTIVE
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/accept", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 4. Complete → COMPLETED (calls transfer)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/complete", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var completed Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &completed))
	assert.Equal(t, StatusCompleted, completed.Status)
	assert.True(t, transferCalled.Load(), "transfer should have been called")

	// 5. Verify outbox has events
	entries, err := outboxStore.FetchUnpublished(ctx, db, 100)
	require.NoError(t, err)
	eventTypes := make([]string, len(entries))
	for i, e := range entries {
		eventTypes[i] = e.EventType
	}
	assert.Contains(t, eventTypes, EventContractCreated)
	assert.Contains(t, eventTypes, EventContractAccepted)
	assert.Contains(t, eventTypes, EventContractCompleted)

	// 6. Verify outbox publisher works with NATS
	publisher := NewOutboxPublisher(outboxStore, db, nc, time.Second)
	require.NoError(t, publisher.PublishBatch(ctx))

	// Verify events published to NATS
	rawConn, err := nats.Connect(nc.conn.ConnectedUrl())
	require.NoError(t, err)
	defer rawConn.Close()

	js, err := jetstream.New(rawConn)
	require.NoError(t, err)

	consumer, err := js.CreateConsumer(ctx, "CONTRACTS", jetstream.ConsumerConfig{
		FilterSubject: "contracts.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	msgs, err := consumer.Fetch(3, jetstream.FetchMaxWait(5*time.Second))
	require.NoError(t, err)

	var received int
	for range msgs.Messages() {
		received++
	}
	assert.Equal(t, 3, received, "should have received 3 events in NATS")
}

func TestIntegration_CompensationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	holdID := uuid.New().String()
	var releaseCalled atomic.Bool
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(HoldResponse{
				ID: holdID, WalletID: uuid.New().String(), Amount: 30000, Status: "active",
			})
		case r.URL.Path == "/api/v1/payments/release":
			releaseCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. Create contract
	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Compensation Test",
		"amount": 30000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// 2. Simulate payment.held → AWAITING_ACCEPT
	require.NoError(t, saga.HandlePaymentHeld(ctx, created.ID))

	// 3. Cancel → DECLINING → release → DECLINED
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/cancel", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var declined Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &declined))
	assert.Equal(t, StatusDeclined, declined.Status)
	assert.True(t, releaseCalled.Load(), "release should have been called")

	// Verify outbox
	entries, err := outboxStore.FetchUnpublished(ctx, db, 100)
	require.NoError(t, err)
	eventTypes := make([]string, len(entries))
	for i, e := range entries {
		eventTypes[i] = e.EventType
	}
	assert.Contains(t, eventTypes, EventContractCreated)
	assert.Contains(t, eventTypes, EventContractDeclined)
}

func TestIntegration_PaymentFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	// Payments returns 422 (insufficient funds)
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": "insufficient funds"})
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, &MySQLMilestoneStore{}, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Failed Payment Test",
		"amount": 100000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Verify contract was created as cancelled
	var contracts []Contract
	rows, err := db.QueryContext(ctx, `SELECT id, status FROM contracts ORDER BY created_at DESC LIMIT 1`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var c Contract
		require.NoError(t, rows.Scan(&c.ID, &c.Status))
		contracts = append(contracts, c)
	}
	require.Len(t, contracts, 1)
	assert.Equal(t, StatusCancelled, contracts[0].Status)
}
