-- +goose Up
CREATE TABLE transactions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id    UUID NOT NULL REFERENCES wallets(id),
    amount       BIGINT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('hold', 'hold_failed', 'release', 'transfer_debit', 'transfer_credit')),
    hold_id      UUID REFERENCES holds(id),
    reference_id UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transactions_wallet ON transactions (wallet_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS transactions;
