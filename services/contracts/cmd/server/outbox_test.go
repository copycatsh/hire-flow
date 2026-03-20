package main

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLOutboxStore_InsertAndFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLOutboxStore{}

	payload, _ := json.Marshal(map[string]string{"test": "data"})
	entry := OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   uuid.New().String(),
		EventType:     EventContractCreated,
		Payload:       payload,
	}

	require.NoError(t, store.Insert(ctx, db, entry))

	entries, err := store.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, entry.ID, entries[0].ID)
	assert.Equal(t, EventContractCreated, entries[0].EventType)
}

func TestMySQLOutboxStore_MarkPublished(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLOutboxStore{}

	payload, _ := json.Marshal(map[string]string{"test": "data"})
	entry := OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   uuid.New().String(),
		EventType:     EventContractCreated,
		Payload:       payload,
	}
	require.NoError(t, store.Insert(ctx, db, entry))

	require.NoError(t, store.MarkPublished(ctx, db, []string{entry.ID}))

	entries, err := store.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
