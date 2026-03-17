package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxEntry struct {
	ID            uuid.UUID  `json:"id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   uuid.UUID  `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	Payload       []byte     `json:"payload"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitzero"`
}

type OutboxStore interface {
	Insert(ctx context.Context, db DBTX, entry OutboxEntry) error
	FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error)
	MarkPublished(ctx context.Context, db DBTX, ids []uuid.UUID) error
}

//
// OutboxPublisher polls unpublished outbox entries and publishes them to NATS.
//
//	┌──────────┐   TX   ┌──────────┐
//	│ Handler   │──────▶│ outbox   │
//	└──────────┘        └────┬─────┘
//	                         │ poll
//	                    ┌────▼─────┐       ┌──────────┐
//	                    │ Publisher │──────▶│  NATS    │
//	                    └──────────┘       └──────────┘
//

type OutboxPublisher struct {
	store    OutboxStore
	pool     *pgxpool.Pool
	nats     *NATSClient
	interval time.Duration
	batch    int
}

func NewOutboxPublisher(store OutboxStore, pool *pgxpool.Pool, nc *NATSClient, interval time.Duration) *OutboxPublisher {
	return &OutboxPublisher{
		store:    store,
		pool:     pool,
		nats:     nc,
		interval: interval,
		batch:    100,
	}
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.publishBatch(ctx); err != nil {
				slog.Error("outbox publish batch", "error", err)
			}
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	entries, err := p.store.FetchUnpublished(ctx, tx, p.batch)
	if err != nil {
		return fmt.Errorf("outbox fetch unpublished: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	var published []uuid.UUID
	for _, e := range entries {
		if err := p.nats.Publish(ctx, e.EventType, e.Payload); err != nil {
			slog.Error("outbox: publish to nats", "error", err, "entry_id", e.ID)
			continue
		}
		published = append(published, e.ID)
	}

	if len(published) > 0 {
		if err := p.store.MarkPublished(ctx, tx, published); err != nil {
			return fmt.Errorf("outbox mark published: %w", err)
		}
	}

	return tx.Commit(ctx)
}
