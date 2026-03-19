package outbox

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX abstracts *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EventPublisher abstracts event publishing (e.g. NATS JetStream).
type EventPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Entry represents a single outbox row.
type Entry struct {
	ID            uuid.UUID  `json:"id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   uuid.UUID  `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	Payload       []byte     `json:"payload"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitzero"`
}

// Store persists and queries outbox entries.
type Store interface {
	Insert(ctx context.Context, db DBTX, entry Entry) error
	FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]Entry, error)
	MarkPublished(ctx context.Context, db DBTX, ids []uuid.UUID) error
}
