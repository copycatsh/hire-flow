package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type PostgresWalletStore struct{}

func (s *PostgresWalletStore) GetByUserID(ctx context.Context, db DBTX, userID uuid.UUID) (Wallet, error) {
	var w Wallet
	err := db.QueryRow(ctx,
		`SELECT w.id, w.user_id, w.balance, w.currency,
		        w.balance - COALESCE(h.held, 0) AS available_balance,
		        w.created_at, w.updated_at
		 FROM wallets w
		 LEFT JOIN (
		     SELECT wallet_id, SUM(amount) AS held
		     FROM holds
		     WHERE status = 'active'
		     GROUP BY wallet_id
		 ) h ON h.wallet_id = w.id
		 WHERE w.user_id = $1`, userID,
	).Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.AvailableBalance, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return Wallet{}, fmt.Errorf("wallet get by user id: %w", err)
	}
	return w, nil
}

func (s *PostgresWalletStore) GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Wallet, error) {
	var w Wallet
	err := db.QueryRow(ctx,
		`SELECT w.id, w.user_id, w.balance, w.currency,
		        w.balance - COALESCE(h.held, 0) AS available_balance,
		        w.created_at, w.updated_at
		 FROM wallets w
		 LEFT JOIN (
		     SELECT wallet_id, SUM(amount) AS held
		     FROM holds
		     WHERE status = 'active'
		     GROUP BY wallet_id
		 ) h ON h.wallet_id = w.id
		 WHERE w.id = $1
		 FOR UPDATE OF w`, id,
	).Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.AvailableBalance, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return Wallet{}, fmt.Errorf("wallet get by id for update: %w", err)
	}
	return w, nil
}

func (s *PostgresWalletStore) UpdateBalance(ctx context.Context, db DBTX, id uuid.UUID, newBalance int64) error {
	_, err := db.Exec(ctx,
		`UPDATE wallets SET balance = $2, updated_at = now() WHERE id = $1`,
		id, newBalance,
	)
	if err != nil {
		return fmt.Errorf("wallet update balance: %w", err)
	}
	return nil
}

func (s *PostgresWalletStore) Seed(ctx context.Context, db DBTX, userID uuid.UUID, balance int64, currency string) (Wallet, error) {
	var w Wallet
	err := db.QueryRow(ctx,
		`INSERT INTO wallets (user_id, balance, currency)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET balance = wallets.balance
		 RETURNING id, user_id, balance, currency, balance AS available_balance, created_at, updated_at`,
		userID, balance, currency,
	).Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.AvailableBalance, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return Wallet{}, fmt.Errorf("wallet seed: %w", err)
	}
	return w, nil
}

func (s *PostgresWalletStore) ListAll(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Query(ctx,
		`SELECT w.id, w.user_id, w.balance, w.currency,
		        w.balance - COALESCE(h.held, 0) AS available_balance,
		        w.created_at, w.updated_at
		 FROM wallets w
		 LEFT JOIN (
		     SELECT wallet_id, SUM(amount) AS held
		     FROM holds
		     WHERE status = 'active'
		     GROUP BY wallet_id
		 ) h ON h.wallet_id = w.id
		 ORDER BY w.created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("wallet list all: %w", err)
	}
	defer rows.Close()
	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.AvailableBalance, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("wallet list scan: %w", err)
		}
		wallets = append(wallets, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wallet list rows: %w", err)
	}
	return wallets, nil
}

func (s *PostgresWalletStore) Count(ctx context.Context, db DBTX) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT COUNT(*) FROM wallets`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("wallet count: %w", err)
	}
	return count, nil
}
