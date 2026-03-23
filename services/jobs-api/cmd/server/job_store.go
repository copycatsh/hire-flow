package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresJobStore struct{}

func (s *PostgresJobStore) Create(ctx context.Context, db DBTX, req CreateJobRequest) (Job, error) {
	var j Job
	err := db.QueryRow(ctx,
		`INSERT INTO jobs (title, description, budget_min, budget_max, client_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, title, description, budget_min, budget_max, status, client_id, created_at, updated_at`,
		req.Title, req.Description, req.BudgetMin, req.BudgetMax, req.ClientID,
	).Scan(&j.ID, &j.Title, &j.Description, &j.BudgetMin, &j.BudgetMax, &j.Status, &j.ClientID, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("job create: %w", err)
	}
	return j, nil
}

func (s *PostgresJobStore) GetByID(ctx context.Context, db DBTX, id uuid.UUID) (Job, error) {
	var j Job
	err := db.QueryRow(ctx,
		`SELECT id, title, description, budget_min, budget_max, status, client_id, created_at, updated_at
		 FROM jobs WHERE id = $1`, id,
	).Scan(&j.ID, &j.Title, &j.Description, &j.BudgetMin, &j.BudgetMax, &j.Status, &j.ClientID, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("job get by id: %w", err)
	}
	return j, nil
}

func (s *PostgresJobStore) List(ctx context.Context, db DBTX, params ListJobsParams) ([]Job, error) {
	var (
		query strings.Builder
		args  []any
		argN  int
	)

	query.WriteString(`SELECT id, title, description, budget_min, budget_max, status, client_id, created_at, updated_at FROM jobs`)

	if params.Status != nil {
		argN++
		query.WriteString(` WHERE status = $` + strconv.Itoa(argN))
		args = append(args, *params.Status)
	}

	query.WriteString(` ORDER BY created_at DESC`)

	if params.Limit > 0 {
		argN++
		query.WriteString(` LIMIT $` + strconv.Itoa(argN))
		args = append(args, params.Limit)
	}

	if params.Offset > 0 {
		argN++
		query.WriteString(` OFFSET $` + strconv.Itoa(argN))
		args = append(args, params.Offset)
	}

	rows, err := db.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("job list: %w", err)
	}

	jobs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Job])
	if err != nil {
		return nil, fmt.Errorf("job list collect: %w", err)
	}
	return jobs, nil
}

func (s *PostgresJobStore) Count(ctx context.Context, db DBTX, params ListJobsParams) (int, error) {
	var (
		query strings.Builder
		args  []any
		argN  int
	)
	query.WriteString(`SELECT COUNT(*) FROM jobs`)
	if params.Status != nil {
		argN++
		query.WriteString(` WHERE status = $` + strconv.Itoa(argN))
		args = append(args, *params.Status)
	}
	var count int
	err := db.QueryRow(ctx, query.String(), args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("job count: %w", err)
	}
	return count, nil
}

func (s *PostgresJobStore) Update(ctx context.Context, db DBTX, id uuid.UUID, req UpdateJobRequest) (Job, error) {
	var j Job
	err := db.QueryRow(ctx,
		`UPDATE jobs SET
			title       = COALESCE($2, title),
			description = COALESCE($3, description),
			budget_min  = COALESCE($4, budget_min),
			budget_max  = COALESCE($5, budget_max),
			status      = COALESCE($6, status),
			updated_at  = now()
		 WHERE id = $1
		 RETURNING id, title, description, budget_min, budget_max, status, client_id, created_at, updated_at`,
		id, req.Title, req.Description, req.BudgetMin, req.BudgetMax, req.Status,
	).Scan(&j.ID, &j.Title, &j.Description, &j.BudgetMin, &j.BudgetMax, &j.Status, &j.ClientID, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("job update: %w", err)
	}
	return j, nil
}
