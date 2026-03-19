package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type PostgresHoldStore struct{}

func (s *PostgresHoldStore) Create(ctx context.Context, db DBTX, walletID uuid.UUID, amount int64, contractID uuid.UUID) (Hold, error) {
	var h Hold
	err := db.QueryRow(ctx,
		`INSERT INTO holds (wallet_id, amount, contract_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, wallet_id, amount, status, contract_id, created_at, expires_at`,
		walletID, amount, contractID,
	).Scan(&h.ID, &h.WalletID, &h.Amount, &h.Status, &h.ContractID, &h.CreatedAt, &h.ExpiresAt)
	if err != nil {
		return Hold{}, fmt.Errorf("hold create: %w", err)
	}
	return h, nil
}

func (s *PostgresHoldStore) GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Hold, error) {
	var h Hold
	err := db.QueryRow(ctx,
		`SELECT id, wallet_id, amount, status, contract_id, created_at, expires_at
		 FROM holds
		 WHERE id = $1
		 FOR UPDATE`, id,
	).Scan(&h.ID, &h.WalletID, &h.Amount, &h.Status, &h.ContractID, &h.CreatedAt, &h.ExpiresAt)
	if err != nil {
		return Hold{}, fmt.Errorf("hold get by id for update: %w", err)
	}
	return h, nil
}

func (s *PostgresHoldStore) UpdateStatus(ctx context.Context, db DBTX, id uuid.UUID, status string) error {
	_, err := db.Exec(ctx,
		`UPDATE holds SET status = $2 WHERE id = $1`,
		id, status,
	)
	if err != nil {
		return fmt.Errorf("hold update status: %w", err)
	}
	return nil
}

func (s *PostgresHoldStore) SumActiveByWallet(ctx context.Context, db DBTX, walletID uuid.UUID) (int64, error) {
	var sum int64
	err := db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM holds WHERE wallet_id = $1 AND status = 'active'`,
		walletID,
	).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("hold sum active by wallet: %w", err)
	}
	return sum, nil
}
