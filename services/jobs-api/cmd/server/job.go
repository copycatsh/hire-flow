package main

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Job struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	BudgetMin   int       `json:"budget_min"`
	BudgetMax   int       `json:"budget_max"`
	Status      string    `json:"status"`
	ClientID    uuid.UUID `json:"client_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateJobRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description,omitzero"`
	BudgetMin   int       `json:"budget_min,omitzero"`
	BudgetMax   int       `json:"budget_max,omitzero"`
	ClientID    uuid.UUID `json:"client_id"`
}

type UpdateJobRequest struct {
	Title       *string `json:"title,omitzero"`
	Description *string `json:"description,omitzero"`
	BudgetMin   *int    `json:"budget_min,omitzero"`
	BudgetMax   *int    `json:"budget_max,omitzero"`
	Status      *string `json:"status,omitzero"`
}

var validJobStatuses = map[string]bool{
	"draft":       true,
	"open":        true,
	"in_progress": true,
	"closed":      true,
}

type ListJobsParams struct {
	Status *string `json:"status,omitzero"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type JobStore interface {
	Create(ctx context.Context, db DBTX, req CreateJobRequest) (Job, error)
	GetByID(ctx context.Context, db DBTX, id uuid.UUID) (Job, error)
	List(ctx context.Context, db DBTX, params ListJobsParams) ([]Job, error)
	Count(ctx context.Context, db DBTX, params ListJobsParams) (int, error)
	Update(ctx context.Context, db DBTX, id uuid.UUID, req UpdateJobRequest) (Job, error)
}
