package main

import (
	"context"
	"fmt"
)

type PostgresTransactionStore struct{}

func (s *PostgresTransactionStore) Create(ctx context.Context, db DBTX, tx Transaction) (Transaction, error) {
	var t Transaction
	err := db.QueryRow(ctx,
		`INSERT INTO transactions (wallet_id, amount, type, hold_id, reference_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, wallet_id, amount, type, hold_id, reference_id, created_at`,
		tx.WalletID, tx.Amount, tx.Type, tx.HoldID, tx.ReferenceID,
	).Scan(&t.ID, &t.WalletID, &t.Amount, &t.Type, &t.HoldID, &t.ReferenceID, &t.CreatedAt)
	if err != nil {
		return Transaction{}, fmt.Errorf("transaction create: %w", err)
	}
	return t, nil
}
