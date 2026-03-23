package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock stores ---

type mockWalletStore struct {
	getByUserIDFn      func(ctx context.Context, db DBTX, userID uuid.UUID) (Wallet, error)
	getByIDForUpdateFn func(ctx context.Context, db DBTX, id uuid.UUID) (Wallet, error)
	updateBalanceFn    func(ctx context.Context, db DBTX, id uuid.UUID, newBalance int64) error
	seedFn             func(ctx context.Context, db DBTX, userID uuid.UUID, balance int64, currency string) (Wallet, error)
	listAllFn          func(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error)
	countFn            func(ctx context.Context, db DBTX) (int, error)
}

func (m *mockWalletStore) GetByUserID(ctx context.Context, db DBTX, userID uuid.UUID) (Wallet, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, db, userID)
	}
	return Wallet{}, nil
}

func (m *mockWalletStore) GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Wallet, error) {
	if m.getByIDForUpdateFn != nil {
		return m.getByIDForUpdateFn(ctx, db, id)
	}
	return Wallet{}, nil
}

func (m *mockWalletStore) UpdateBalance(ctx context.Context, db DBTX, id uuid.UUID, newBalance int64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, db, id, newBalance)
	}
	return nil
}

func (m *mockWalletStore) Seed(ctx context.Context, db DBTX, userID uuid.UUID, balance int64, currency string) (Wallet, error) {
	if m.seedFn != nil {
		return m.seedFn(ctx, db, userID, balance, currency)
	}
	return Wallet{}, nil
}

func (m *mockWalletStore) ListAll(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, db, limit, offset)
	}
	return nil, nil
}

func (m *mockWalletStore) Count(ctx context.Context, db DBTX) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, db)
	}
	return 0, nil
}

type mockHoldStore struct {
	createFn           func(ctx context.Context, db DBTX, walletID uuid.UUID, amount int64, contractID uuid.UUID) (Hold, error)
	getByIDForUpdateFn func(ctx context.Context, db DBTX, id uuid.UUID) (Hold, error)
	updateStatusFn     func(ctx context.Context, db DBTX, id uuid.UUID, status string) error
	sumActiveByWalletFn func(ctx context.Context, db DBTX, walletID uuid.UUID) (int64, error)
}

func (m *mockHoldStore) Create(ctx context.Context, db DBTX, walletID uuid.UUID, amount int64, contractID uuid.UUID) (Hold, error) {
	if m.createFn != nil {
		return m.createFn(ctx, db, walletID, amount, contractID)
	}
	return Hold{}, nil
}

func (m *mockHoldStore) GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Hold, error) {
	if m.getByIDForUpdateFn != nil {
		return m.getByIDForUpdateFn(ctx, db, id)
	}
	return Hold{}, nil
}

func (m *mockHoldStore) UpdateStatus(ctx context.Context, db DBTX, id uuid.UUID, status string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, db, id, status)
	}
	return nil
}

func (m *mockHoldStore) SumActiveByWallet(ctx context.Context, db DBTX, walletID uuid.UUID) (int64, error) {
	if m.sumActiveByWalletFn != nil {
		return m.sumActiveByWalletFn(ctx, db, walletID)
	}
	return 0, nil
}

type mockTransactionStore struct {
	createFn func(ctx context.Context, db DBTX, tx Transaction) (Transaction, error)
}

func (m *mockTransactionStore) Create(ctx context.Context, db DBTX, tx Transaction) (Transaction, error) {
	if m.createFn != nil {
		return m.createFn(ctx, db, tx)
	}
	return Transaction{}, nil
}

type mockOutboxStore struct {
	insertFn           func(ctx context.Context, db outbox.DBTX, entry outbox.Entry) error
	fetchUnpublishedFn func(ctx context.Context, db outbox.DBTX, limit int) ([]outbox.Entry, error)
	markPublishedFn    func(ctx context.Context, db outbox.DBTX, ids []uuid.UUID) error
}

func (m *mockOutboxStore) Insert(ctx context.Context, db outbox.DBTX, entry outbox.Entry) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, db, entry)
	}
	return nil
}

func (m *mockOutboxStore) FetchUnpublished(ctx context.Context, db outbox.DBTX, limit int) ([]outbox.Entry, error) {
	if m.fetchUnpublishedFn != nil {
		return m.fetchUnpublishedFn(ctx, db, limit)
	}
	return nil, nil
}

func (m *mockOutboxStore) MarkPublished(ctx context.Context, db outbox.DBTX, ids []uuid.UUID) error {
	if m.markPublishedFn != nil {
		return m.markPublishedFn(ctx, db, ids)
	}
	return nil
}

// --- Helpers ---

func setupTestRouter(h *PaymentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

// --- Tests ---

func TestGetWallet_NotFound(t *testing.T) {
	userID := uuid.New()
	h := &PaymentHandler{
		wallets: &mockWalletStore{
			getByUserIDFn: func(_ context.Context, _ DBTX, _ uuid.UUID) (Wallet, error) {
				return Wallet{}, pgx.ErrNoRows
			},
		},
	}
	r := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/wallet/"+userID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "wallet not found", body["error"])
}

func TestGetWallet_InvalidUUID(t *testing.T) {
	h := &PaymentHandler{
		wallets: &mockWalletStore{},
	}
	r := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/wallet/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid user_id", body["error"])
}

func TestHoldFunds_MissingWalletID(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{"amount": 100, "contract_id": "` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "wallet_id is required", resp["error"])
}

func TestHoldFunds_ZeroAmount(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{"wallet_id": "` + uuid.New().String() + `", "amount": 0, "contract_id": "` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "amount must be positive", resp["error"])
}

func TestHoldFunds_MissingContractID(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{"wallet_id": "` + uuid.New().String() + `", "amount": 100}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/hold", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "contract_id is required", resp["error"])
}

func TestReleaseFunds_MissingHoldID(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/release", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "hold_id is required", resp["error"])
}

func TestTransferFunds_MissingHoldID(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{"recipient_wallet_id": "` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "hold_id is required", resp["error"])
}

func TestTransferFunds_MissingRecipientWalletID(t *testing.T) {
	h := &PaymentHandler{
		wallets:      &mockWalletStore{},
		holds:        &mockHoldStore{},
		transactions: &mockTransactionStore{},
		outbox:       &mockOutboxStore{},
	}
	r := setupTestRouter(h)

	body := `{"hold_id": "` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/transfer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "recipient_wallet_id is required", resp["error"])
}

func TestListWallets_Empty(t *testing.T) {
	h := &PaymentHandler{
		wallets: &mockWalletStore{
			listAllFn: func(_ context.Context, _ DBTX, _, _ int) ([]Wallet, error) {
				return nil, nil
			},
			countFn: func(_ context.Context, _ DBTX) (int, error) {
				return 0, nil
			},
		},
	}
	r := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/wallets", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body ListResponse[Wallet]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Items)
	assert.Equal(t, 0, body.Total)
}

func TestListWallets_InvalidLimit(t *testing.T) {
	h := &PaymentHandler{
		wallets: &mockWalletStore{},
	}
	r := setupTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/wallets?limit=abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid limit", body["error"])
}
