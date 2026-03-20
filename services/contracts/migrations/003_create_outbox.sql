-- +goose Up
CREATE TABLE outbox (
    id             CHAR(36) PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   CHAR(36) NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSON NOT NULL,
    created_at     TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    published_at   TIMESTAMP(6) NULL DEFAULT NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (published_at, created_at);

-- +goose Down
DROP TABLE IF EXISTS outbox;
