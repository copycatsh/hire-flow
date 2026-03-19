package main

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Hold struct {
	ID         uuid.UUID  `json:"id"`
	WalletID   uuid.UUID  `json:"wallet_id"`
	Amount     int64      `json:"amount"`
	Status     string     `json:"status"`
	ContractID uuid.UUID  `json:"contract_id"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitzero"`
}

type HoldRequest struct {
	WalletID   uuid.UUID `json:"wallet_id"`
	Amount     int64     `json:"amount"`
	ContractID uuid.UUID `json:"contract_id"`
}

type ReleaseRequest struct {
	HoldID uuid.UUID `json:"hold_id"`
}

type TransferRequest struct {
	HoldID            uuid.UUID `json:"hold_id"`
	RecipientWalletID uuid.UUID `json:"recipient_wallet_id"`
}

type HoldStore interface {
	Create(ctx context.Context, db DBTX, walletID uuid.UUID, amount int64, contractID uuid.UUID) (Hold, error)
	GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (Hold, error)
	UpdateStatus(ctx context.Context, db DBTX, id uuid.UUID, status string) error
	SumActiveByWallet(ctx context.Context, db DBTX, walletID uuid.UUID) (int64, error)
}
