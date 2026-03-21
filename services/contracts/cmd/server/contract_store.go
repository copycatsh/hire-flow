package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrContractNotFound = errors.New("contract not found")
var ErrStatusConflict = errors.New("contract status conflict")

type MySQLContractStore struct{}

func (s *MySQLContractStore) Create(ctx context.Context, db DBTX, c Contract) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO contracts (id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ClientID, c.FreelancerID, c.Title, c.Description, c.Amount, c.Currency, c.Status, c.ClientWalletID, c.FreelancerWalletID,
	)
	if err != nil {
		return fmt.Errorf("contract create: %w", err)
	}
	return nil
}

func (s *MySQLContractStore) GetByID(ctx context.Context, db DBTX, id string) (Contract, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id, hold_id, created_at, updated_at
		 FROM contracts WHERE id = ?`, id,
	)

	var c Contract
	err := row.Scan(&c.ID, &c.ClientID, &c.FreelancerID, &c.Title, &c.Description, &c.Amount, &c.Currency, &c.Status, &c.ClientWalletID, &c.FreelancerWalletID, &c.HoldID, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Contract{}, ErrContractNotFound
	}
	if err != nil {
		return Contract{}, fmt.Errorf("contract get by id: %w", err)
	}
	return c, nil
}

func (s *MySQLContractStore) UpdateStatus(ctx context.Context, db DBTX, id string, from string, to string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE contracts SET status = ? WHERE id = ? AND status = ?`,
		to, id, from,
	)
	if err != nil {
		return fmt.Errorf("contract update status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("contract update status rows: %w", err)
	}
	if rows == 0 {
		return ErrStatusConflict
	}
	return nil
}

func (s *MySQLContractStore) List(ctx context.Context, db DBTX, filter ListFilter) ([]Contract, error) {
	query := `SELECT id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id, hold_id, created_at, updated_at FROM contracts WHERE 1=1`
	args := []any{}

	if filter.ClientID != "" {
		query += " AND client_id = ?"
		args = append(args, filter.ClientID)
	}
	if filter.FreelancerID != "" {
		query += " AND freelancer_id = ?"
		args = append(args, filter.FreelancerID)
	}

	query += " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("contract list: %w", err)
	}
	defer rows.Close()

	var contracts []Contract
	for rows.Next() {
		var c Contract
		if err := rows.Scan(&c.ID, &c.ClientID, &c.FreelancerID, &c.Title, &c.Description, &c.Amount, &c.Currency, &c.Status, &c.ClientWalletID, &c.FreelancerWalletID, &c.HoldID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("contract list scan: %w", err)
		}
		contracts = append(contracts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contract list rows: %w", err)
	}
	return contracts, nil
}

func (s *MySQLContractStore) SetHoldID(ctx context.Context, db DBTX, id string, holdID string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE contracts SET hold_id = ? WHERE id = ?`,
		holdID, id,
	)
	if err != nil {
		return fmt.Errorf("contract set hold_id: %w", err)
	}
	return nil
}
