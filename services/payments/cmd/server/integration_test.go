package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/copycatsh/hire-flow/services/payments/migrations"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	pgContainer, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("test_payments"),
		tcpg.WithUsername("test"),
		tcpg.WithPassword("test"),
		tcpg.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "."))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool
}

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

func setupHandler(pool *pgxpool.Pool) *PaymentHandler {
	return &PaymentHandler{
		pool:         pool,
		wallets:      &PostgresWalletStore{},
		holds:        &PostgresHoldStore{},
		transactions: &PostgresTransactionStore{},
		outbox:       &outbox.PostgresStore{},
	}
}

func seedWallet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, balanceCents int64) Wallet {
	t.Helper()
	ws := &PostgresWalletStore{}
	w, err := ws.Seed(ctx, pool, userID, balanceCents, "USD")
	require.NoError(t, err)
	return w
}

func newTestRouter(h *PaymentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func TestHoldFunds_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)
	contractID := uuid.New()

	body := `{"wallet_id":"` + w.ID.String() + `","amount":20000,"contract_id":"` + contractID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))
	assert.Equal(t, w.ID, hold.WalletID)
	assert.Equal(t, int64(20000), hold.Amount)
	assert.Equal(t, "active", hold.Status)
	assert.Equal(t, contractID, hold.ContractID)

	// Verify transaction record
	var txCount int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE hold_id = $1`, hold.ID).Scan(&txCount)
	require.NoError(t, err)
	assert.Equal(t, 1, txCount)

	// Verify outbox entry
	var eventType string
	err = pool.QueryRow(ctx, `SELECT event_type FROM outbox WHERE aggregate_id = $1`, hold.ID).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, EventPaymentHeld, eventType)

	// Verify available balance decreased
	ws := &PostgresWalletStore{}
	updated, err := ws.GetByUserID(ctx, pool, w.UserID)
	require.NoError(t, err)
	assert.Equal(t, int64(50000), updated.Balance)
	assert.Equal(t, int64(30000), updated.AvailableBalance)
}

func TestHoldFunds_InsufficientFunds_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 10000)

	body := `{"wallet_id":"` + w.ID.String() + `","amount":50000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "insufficient funds", resp["error"])

	// Verify payment.failed in outbox
	var eventType string
	err := pool.QueryRow(ctx, `SELECT event_type FROM outbox ORDER BY created_at DESC LIMIT 1`).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, EventPaymentFailed, eventType)
}

func TestReleaseFunds_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	// Create hold
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":20000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// Release
	releaseBody := `{"hold_id":"` + hold.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/release", strings.NewReader(releaseBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var released Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &released))
	assert.Equal(t, "released", released.Status)

	// Verify outbox has payment.released
	var eventType string
	err := pool.QueryRow(ctx,
		`SELECT event_type FROM outbox WHERE aggregate_id = $1 AND event_type = $2`,
		hold.ID, EventPaymentReleased,
	).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, EventPaymentReleased, eventType)
}

func TestReleaseFunds_DoubleRelease_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	// Create hold
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":20000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// First release
	releaseBody := `{"hold_id":"` + hold.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/release", strings.NewReader(releaseBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Second release -> 409
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/release", strings.NewReader(releaseBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "hold is not active", resp["error"])
}

func TestTransferFunds_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	clientWallet := seedWallet(t, ctx, pool, uuid.New(), 50000)
	freelancerWallet := seedWallet(t, ctx, pool, uuid.New(), 10000)

	// Create hold on client wallet
	holdBody := `{"wallet_id":"` + clientWallet.ID.String() + `","amount":30000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// Transfer
	transferBody := `{"hold_id":"` + hold.ID.String() + `","recipient_wallet_id":"` + freelancerWallet.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(transferBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Verify client balance: 50000 - 30000 = 20000
	ws := &PostgresWalletStore{}
	updatedClient, err := ws.GetByUserID(ctx, pool, clientWallet.UserID)
	require.NoError(t, err)
	assert.Equal(t, int64(20000), updatedClient.Balance)

	// Verify freelancer balance: 10000 + 30000 = 40000
	updatedFreelancer, err := ws.GetByUserID(ctx, pool, freelancerWallet.UserID)
	require.NoError(t, err)
	assert.Equal(t, int64(40000), updatedFreelancer.Balance)

	// Verify hold status = transferred
	var holdStatus string
	err = pool.QueryRow(ctx, `SELECT status FROM holds WHERE id = $1`, hold.ID).Scan(&holdStatus)
	require.NoError(t, err)
	assert.Equal(t, "transferred", holdStatus)

	// Verify 2 transaction records for the transfer (debit + credit)
	var txCount int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM transactions WHERE hold_id = $1 AND type IN ('transfer_debit', 'transfer_credit')`,
		hold.ID,
	).Scan(&txCount)
	require.NoError(t, err)
	assert.Equal(t, 2, txCount)

	// Verify outbox has payment.transferred
	var eventType string
	err = pool.QueryRow(ctx,
		`SELECT event_type FROM outbox WHERE aggregate_id = $1 AND event_type = $2`,
		hold.ID, EventPaymentTransferred,
	).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, EventPaymentTransferred, eventType)
}

func TestTransferFunds_HoldNotActive_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	clientWallet := seedWallet(t, ctx, pool, uuid.New(), 50000)
	freelancerWallet := seedWallet(t, ctx, pool, uuid.New(), 10000)

	// Create hold
	holdBody := `{"wallet_id":"` + clientWallet.ID.String() + `","amount":20000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// Release the hold
	releaseBody := `{"hold_id":"` + hold.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/release", strings.NewReader(releaseBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Try transfer -> 409
	transferBody := `{"hold_id":"` + hold.ID.String() + `","recipient_wallet_id":"` + freelancerWallet.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(transferBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "hold is not active", resp["error"])
}

func TestGetWallet_WithAvailableBalance_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	userID := uuid.New()
	w := seedWallet(t, ctx, pool, userID, 50000)

	// Create a hold to reduce available balance
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":15000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Get wallet
	req = httptest.NewRequest(http.MethodGet, "/api/v1/payments/wallet/"+userID.String(), nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var wallet Wallet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &wallet))
	assert.Equal(t, int64(50000), wallet.Balance)
	assert.Equal(t, int64(35000), wallet.AvailableBalance)
}

func TestOutboxPublisher_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	nc := setupNATS(t, ctx)

	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	// Create hold via HTTP (inserts outbox entry)
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":10000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Subscribe NATS consumer to EventPaymentHeld
	rawConn, err := nats.Connect(nc.conn.ConnectedUrl())
	require.NoError(t, err)
	defer rawConn.Close()

	js, err := jetstream.New(rawConn)
	require.NoError(t, err)

	consumer, err := js.CreateConsumer(ctx, "PAYMENTS", jetstream.ConsumerConfig{
		FilterSubject: EventPaymentHeld,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	// Run publisher batch
	outboxStore := &outbox.PostgresStore{}
	publisher := outbox.NewPublisher(outboxStore, pool, nc, time.Second)
	require.NoError(t, publisher.PublishBatch(ctx))

	// Verify message received
	msg, err := consumer.Next(jetstream.FetchMaxWait(5 * time.Second))
	require.NoError(t, err)
	require.NotNil(t, msg)

	var receivedHold Hold
	require.NoError(t, json.Unmarshal(msg.Data(), &receivedHold))
	assert.Equal(t, w.ID, receivedHold.WalletID)
	assert.Equal(t, int64(10000), receivedHold.Amount)

	// Verify outbox marked published
	var publishedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT published_at FROM outbox WHERE event_type = $1 ORDER BY created_at DESC LIMIT 1`,
		EventPaymentHeld,
	).Scan(&publishedAt)
	require.NoError(t, err)
	assert.NotNil(t, publishedAt)
}

func TestTransactionRollback_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)
	hs := &PostgresHoldStore{}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	hold, err := hs.Create(ctx, tx, w.ID, 10000, uuid.New())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, hold.ID)

	require.NoError(t, tx.Rollback(ctx))

	// Verify no holds exist
	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM holds WHERE id = $1`, hold.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestConcurrentHolds_NoOverdraft_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	var successCount atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			gin.SetMode(gin.TestMode)
			rr := gin.New()
			h.RegisterRoutes(rr)

			body := `{"wallet_id":"` + w.ID.String() + `","amount":40000,"contract_id":"` + uuid.New().String() + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			rr.ServeHTTP(rec, req)

			if rec.Code == http.StatusCreated {
				successCount.Add(1)
			}
		})
	}
	wg.Wait()

	assert.Equal(t, int32(1), successCount.Load())

	// Verify total active holds <= wallet balance
	var totalHeld int64
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM holds WHERE wallet_id = $1 AND status = 'active'`,
		w.ID,
	).Scan(&totalHeld)
	require.NoError(t, err)
	assert.Equal(t, int64(40000), totalHeld, "exactly one hold of 40000 should exist")

	// Verify wallet balance unchanged (holds don't modify balance)
	var balance int64
	err = pool.QueryRow(ctx, `SELECT balance FROM wallets WHERE id = $1`, w.ID).Scan(&balance)
	require.NoError(t, err)
	assert.Equal(t, int64(50000), balance, "wallet balance should be unchanged")
}

func TestTransferFunds_TransferToSelf_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	// Create hold
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":20000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// Try transfer to same wallet → 400
	transferBody := `{"hold_id":"` + hold.ID.String() + `","recipient_wallet_id":"` + w.ID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(transferBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "cannot transfer to source wallet", resp["error"])
}

func TestTransferFunds_RecipientNotFound_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	h := setupHandler(pool)
	r := newTestRouter(h)

	w := seedWallet(t, ctx, pool, uuid.New(), 50000)

	// Create hold
	holdBody := `{"wallet_id":"` + w.ID.String() + `","amount":20000,"contract_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(holdBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var hold Hold
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &hold))

	// Transfer to non-existent wallet → 404
	fakeWalletID := uuid.New()
	transferBody := `{"hold_id":"` + hold.ID.String() + `","recipient_wallet_id":"` + fakeWalletID.String() + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(transferBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "recipient wallet not found", resp["error"])
}
