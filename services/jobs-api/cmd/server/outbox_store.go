package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresOutboxStore struct{}

func (s *PostgresOutboxStore) Insert(ctx context.Context, db DBTX, entry OutboxEntry) error {
	_, err := db.Exec(ctx,
		`INSERT INTO outbox (aggregate_type, aggregate_id, event_type, payload)
		 VALUES ($1, $2, $3, $4)`,
		entry.AggregateType, entry.AggregateID, entry.EventType, entry.Payload,
	)
	if err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}
	return nil
}

func (s *PostgresOutboxStore) FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error) {
	rows, err := db.Query(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at, published_at
		 FROM outbox
		 WHERE published_at IS NULL
		 ORDER BY created_at
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox fetch unpublished: %w", err)
	}

	entries, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OutboxEntry])
	if err != nil {
		return nil, fmt.Errorf("outbox fetch unpublished collect: %w", err)
	}
	return entries, nil
}

func (s *PostgresOutboxStore) MarkPublished(ctx context.Context, db DBTX, ids []uuid.UUID) error {
	_, err := db.Exec(ctx,
		`UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids,
	)
	if err != nil {
		return fmt.Errorf("outbox mark published: %w", err)
	}
	return nil
}
