package main

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Wallet struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	Balance          int64     `json:"balance"`
	Currency         string    `json:"currency"`
	AvailableBalance int64     `json:"available_balance"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type WalletStore interface {
	GetByUserID(ctx context.Context, db DBTX, userID uuid.UUID) (Wallet, error)
	GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Wallet, error)
	UpdateBalance(ctx context.Context, db DBTX, id uuid.UUID, newBalance int64) error
	Seed(ctx context.Context, db DBTX, userID uuid.UUID, balance int64, currency string) (Wallet, error)
	ListAll(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error)
	Count(ctx context.Context, db DBTX) (int, error)
}
