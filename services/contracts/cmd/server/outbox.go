package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type OutboxEntry struct {
	ID            string     `json:"id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	Payload       []byte     `json:"payload"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitzero"`
}

// OutboxStore persists and queries outbox entries in MySQL.
type OutboxStore interface {
	Insert(ctx context.Context, db DBTX, entry OutboxEntry) error
	FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error)
	MarkPublished(ctx context.Context, db DBTX, ids []string) error
}

// EventPublisher sends events to a message broker.
type EventPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

type MySQLOutboxStore struct{}

func (s *MySQLOutboxStore) Insert(ctx context.Context, db DBTX, entry OutboxEntry) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.AggregateType, entry.AggregateID, entry.EventType, entry.Payload,
	)
	if err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}
	return nil
}

func (s *MySQLOutboxStore) FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at, published_at
		 FROM outbox
		 WHERE published_at IS NULL
		 ORDER BY created_at
		 LIMIT ?
		 FOR UPDATE SKIP LOCKED`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox fetch unpublished: %w", err)
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &e.Payload, &e.CreatedAt, &e.PublishedAt); err != nil {
			return nil, fmt.Errorf("outbox scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *MySQLOutboxStore) MarkPublished(ctx context.Context, db DBTX, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	_, err := db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE outbox SET published_at = CURRENT_TIMESTAMP(6) WHERE id IN (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return fmt.Errorf("outbox mark published: %w", err)
	}
	return nil
}
