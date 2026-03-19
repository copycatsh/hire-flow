package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

//
// Publisher polls unpublished outbox entries and publishes them via EventPublisher.
//
//	┌──────────┐   TX   ┌──────────┐
//	│ Handler   │──────▶│ outbox   │
//	└──────────┘        └────┬─────┘
//	                         │ poll
//	                    ┌────▼─────┐       ┌──────────┐
//	                    │ Publisher │──────▶│  NATS    │
//	                    └──────────┘       └──────────┘
//

type Publisher struct {
	store    Store
	pool     *pgxpool.Pool
	pub      EventPublisher
	interval time.Duration
	batch    int
}

func NewPublisher(store Store, pool *pgxpool.Pool, pub EventPublisher, interval time.Duration) *Publisher {
	return &Publisher{
		store:    store,
		pool:     pool,
		pub:      pub,
		interval: interval,
		batch:    100,
	}
}

func (p *Publisher) Run(ctx context.Context) {
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

func (p *Publisher) PublishBatch(ctx context.Context) error {
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

	return tx.Commit(ctx)
}
