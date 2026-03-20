package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// OutboxPublisher polls the MySQL outbox table and publishes entries to NATS.
type OutboxPublisher struct {
	store    OutboxStore
	db       *sql.DB
	pub      EventPublisher
	interval time.Duration
	batch    int
}

func NewOutboxPublisher(store OutboxStore, db *sql.DB, pub EventPublisher, interval time.Duration) *OutboxPublisher {
	return &OutboxPublisher{
		store:    store,
		db:       db,
		pub:      pub,
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
			if err := p.PublishBatch(ctx); err != nil {
				slog.Error("outbox publish batch", "error", err)
			}
		}
	}
}

func (p *OutboxPublisher) PublishBatch(ctx context.Context) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("outbox begin tx: %w", err)
	}
	defer tx.Rollback()

	entries, err := p.store.FetchUnpublished(ctx, tx, p.batch)
	if err != nil {
		return fmt.Errorf("outbox fetch unpublished: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	var published []string
	for _, e := range entries {
		if err := p.pub.Publish(ctx, e.EventType, e.Payload); err != nil {
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

	return tx.Commit()
}
