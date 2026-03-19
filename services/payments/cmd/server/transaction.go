package main

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Transaction struct {
	ID          uuid.UUID  `json:"id"`
	WalletID    uuid.UUID  `json:"wallet_id"`
	Amount      int64      `json:"amount"`
	Type        string     `json:"type"`
	HoldID      *uuid.UUID `json:"hold_id,omitzero"`
	ReferenceID *uuid.UUID `json:"reference_id,omitzero"`
	CreatedAt   time.Time  `json:"created_at"`
}

type TransactionStore interface {
	Create(ctx context.Context, db DBTX, tx Transaction) (Transaction, error)
}
