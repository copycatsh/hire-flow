package main

import (
	"context"
	"time"
)

type Milestone struct {
	ID          string    `json:"id"`
	ContractID  string    `json:"contract_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Position    int       `json:"position"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MilestoneStore interface {
	CreateBatch(ctx context.Context, db DBTX, milestones []Milestone) error
	ListByContract(ctx context.Context, db DBTX, contractID string) ([]Milestone, error)
}
