-- +goose Up
CREATE TABLE jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    budget_min  INT  NOT NULL DEFAULT 0,
    budget_max  INT  NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'draft',
    client_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jobs_status ON jobs (status);
CREATE INDEX idx_jobs_client_id ON jobs (client_id);

-- +goose Down
DROP TABLE IF EXISTS jobs;
