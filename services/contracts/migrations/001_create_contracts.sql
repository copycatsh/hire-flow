-- +goose Up
CREATE TABLE contracts (
    id                CHAR(36) PRIMARY KEY,
    client_id         CHAR(36) NOT NULL,
    freelancer_id     CHAR(36) NOT NULL,
    title             VARCHAR(500) NOT NULL,
    description       TEXT NOT NULL DEFAULT (''),
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          VARCHAR(3) NOT NULL DEFAULT 'USD',
    status            VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    client_wallet_id  CHAR(36) NOT NULL,
    freelancer_wallet_id CHAR(36) NOT NULL,
    hold_id           CHAR(36),
    created_at        TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at        TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    INDEX idx_contracts_client (client_id),
    INDEX idx_contracts_freelancer (freelancer_id),
    INDEX idx_contracts_status (status)
);

-- +goose Down
DROP TABLE IF EXISTS contracts;
