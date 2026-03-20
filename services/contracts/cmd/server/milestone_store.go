package main

import (
	"context"
	"fmt"
)

type MySQLMilestoneStore struct{}

func (s *MySQLMilestoneStore) CreateBatch(ctx context.Context, db DBTX, milestones []Milestone) error {
	for _, m := range milestones {
		_, err := db.ExecContext(ctx,
			`INSERT INTO milestones (id, contract_id, title, description, amount, position, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.ContractID, m.Title, m.Description, m.Amount, m.Position, m.Status,
		)
		if err != nil {
			return fmt.Errorf("milestone create: %w", err)
		}
	}
	return nil
}

func (s *MySQLMilestoneStore) ListByContract(ctx context.Context, db DBTX, contractID string) ([]Milestone, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, contract_id, title, description, amount, position, status, created_at, updated_at
		 FROM milestones WHERE contract_id = ? ORDER BY position`, contractID,
	)
	if err != nil {
		return nil, fmt.Errorf("milestone list: %w", err)
	}
	defer rows.Close()

	var milestones []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.ContractID, &m.Title, &m.Description, &m.Amount, &m.Position, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("milestone scan: %w", err)
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}
