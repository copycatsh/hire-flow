-- +goose Up
CREATE TABLE holds (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id   UUID NOT NULL REFERENCES wallets(id),
    amount      BIGINT NOT NULL CHECK (amount > 0),
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'released', 'transferred')),
    contract_id UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ
);

CREATE INDEX idx_holds_wallet_active ON holds (wallet_id) WHERE status = 'active';

-- +goose Down
DROP TABLE IF EXISTS holds;
