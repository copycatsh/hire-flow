-- +goose Up
CREATE TABLE milestones (
    id          CHAR(36) PRIMARY KEY,
    contract_id CHAR(36) NOT NULL,
    title       VARCHAR(500) NOT NULL,
    description TEXT NOT NULL DEFAULT (''),
    amount      BIGINT NOT NULL CHECK (amount > 0),
    position    INT NOT NULL DEFAULT 0,
    status      VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at  TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at  TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    FOREIGN KEY (contract_id) REFERENCES contracts(id),
    INDEX idx_milestones_contract (contract_id)
);

-- +goose Down
DROP TABLE IF EXISTS milestones;
