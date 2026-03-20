package main

import (
	"context"
	"time"
)

// Contract status constants — saga states embedded in contract lifecycle.
//
//	                    ┌──────────┐
//	       POST /contracts        │
//	                    ▼         │
//	               ┌─────────┐   │
//	               │ PENDING  │   │
//	               └────┬─────┘   │
//	                    │ HTTP: POST /payments/hold
//	                    ▼
//	            ┌──────────────┐
//	            │ HOLD_PENDING │
//	            └──────┬───────┘
//	        ┌──────────┼──────────┐
//	  payment.failed   │   payment.held
//	        ▼          │          ▼
//	  ┌───────────┐    │   ┌────────────────┐
//	  │ CANCELLED │    │   │ AWAITING_ACCEPT │
//	  └───────────┘    │   └────┬────────────┘
//	              ┌────┴────┐   │
//	        PUT /accept  PUT /cancel
//	              ▼         ▼
//	        ┌────────┐  ┌───────────┐
//	        │ ACTIVE │  │ DECLINING │─── release ──▶ DECLINED
//	        └───┬────┘  └───────────┘
//	     PUT /complete
//	            ▼
//	     ┌────────────┐
//	     │ COMPLETING │─── transfer ──▶ COMPLETED
//	     └────────────┘
const (
	StatusPending        = "PENDING"
	StatusHoldPending    = "HOLD_PENDING"
	StatusAwaitingAccept = "AWAITING_ACCEPT"
	StatusActive         = "ACTIVE"
	StatusCompleting     = "COMPLETING"
	StatusCompleted      = "COMPLETED"
	StatusDeclining      = "DECLINING"
	StatusDeclined       = "DECLINED"
	StatusCancelled      = "CANCELLED"
)

type Contract struct {
	ID                 string    `json:"id"`
	ClientID           string    `json:"client_id"`
	FreelancerID       string    `json:"freelancer_id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Amount             int64     `json:"amount"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	ClientWalletID     string    `json:"client_wallet_id"`
	FreelancerWalletID string    `json:"freelancer_wallet_id"`
	HoldID             *string   `json:"hold_id,omitzero"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CreateContractRequest struct {
	ClientID           string          `json:"client_id"`
	FreelancerID       string          `json:"freelancer_id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Amount             int64           `json:"amount"`
	ClientWalletID     string          `json:"client_wallet_id"`
	FreelancerWalletID string          `json:"freelancer_wallet_id"`
	Milestones         []MilestoneSpec `json:"milestones"`
}

type MilestoneSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"`
	Position    int    `json:"position"`
}

type ContractStore interface {
	Create(ctx context.Context, db DBTX, c Contract) error
	GetByID(ctx context.Context, db DBTX, id string) (Contract, error)
	UpdateStatus(ctx context.Context, db DBTX, id string, from string, to string) error
	SetHoldID(ctx context.Context, db DBTX, id string, holdID string) error
}
