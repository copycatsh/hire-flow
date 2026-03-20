package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLContractStore_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Build API",
		Description:        "REST API development",
		Amount:             50000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}

	require.NoError(t, store.Create(ctx, db, c))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, c.Title, got.Title)
	assert.Equal(t, c.Amount, got.Amount)
	assert.Equal(t, StatusPending, got.Status)
	assert.Nil(t, got.HoldID)
}

func TestMySQLContractStore_UpdateStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test Contract",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, store.Create(ctx, db, c))

	// Valid transition
	require.NoError(t, store.UpdateStatus(ctx, db, c.ID, StatusPending, StatusHoldPending))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusHoldPending, got.Status)

	// Wrong "from" status → error
	err = store.UpdateStatus(ctx, db, c.ID, StatusPending, StatusActive)
	assert.Error(t, err)
}

func TestMySQLContractStore_SetHoldID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test Contract",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, store.Create(ctx, db, c))

	holdID := uuid.New().String()
	require.NoError(t, store.SetHoldID(ctx, db, c.ID, holdID))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got.HoldID)
	assert.Equal(t, holdID, *got.HoldID)
}

func TestMySQLContractStore_GetByID_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	_, err := store.GetByID(ctx, db, uuid.New().String())
	assert.ErrorIs(t, err, ErrContractNotFound)
}
