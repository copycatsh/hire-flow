package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// SagaOrchestrator coordinates the contract lifecycle across services.
//
// Forward flow:
//
//	CreateContract → PENDING → call Payments hold → HOLD_PENDING
//	payment.held event → AWAITING_ACCEPT
//	AcceptContract → ACTIVE (+ outbox: contract.accepted)
//	CompleteContract → COMPLETING → call Payments transfer → COMPLETED (+ outbox: contract.completed)
//
// Compensation flow:
//
//	payment.failed event → CANCELLED (+ outbox: contract.cancelled)
//	CancelContract → DECLINING → call Payments release → DECLINED (+ outbox: contract.declined)
//
// Retry: CompleteContract and CancelContract handle both fresh calls and retries
// from COMPLETING/DECLINING states (idempotent).

// PaymentsService defines the payments operations needed by the saga.
type PaymentsService interface {
	HoldFunds(ctx context.Context, req HoldRequest) (HoldResponse, error)
	ReleaseFunds(ctx context.Context, holdID string) error
	TransferFunds(ctx context.Context, holdID string, recipientWalletID string) error
}

type SagaOrchestrator struct {
	db         *sql.DB
	contracts  ContractStore
	milestones MilestoneStore
	outbox     OutboxStore
	payments   PaymentsService
}

func NewSagaOrchestrator(db *sql.DB, contracts ContractStore, milestones MilestoneStore, outbox OutboxStore, payments PaymentsService) *SagaOrchestrator {
	return &SagaOrchestrator{
		db:         db,
		contracts:  contracts,
		milestones: milestones,
		outbox:     outbox,
		payments:   payments,
	}
}

// CreateContract creates a contract and immediately calls Payments to hold funds.
// Returns the contract in HOLD_PENDING status on success.
func (s *SagaOrchestrator) CreateContract(ctx context.Context, c Contract, milestones []Milestone) (Contract, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: begin tx: %w", err)
	}
	defer tx.Rollback()

	c.ID = uuid.New().String()
	c.Status = StatusPending
	c.Currency = "USD"

	if err := s.contracts.Create(ctx, tx, c); err != nil {
		return Contract{}, fmt.Errorf("saga create: insert contract: %w", err)
	}

	if len(milestones) > 0 {
		for i := range milestones {
			milestones[i].ID = uuid.New().String()
			milestones[i].ContractID = c.ID
			milestones[i].Status = "PENDING"
		}
		if err := s.milestones.CreateBatch(ctx, tx, milestones); err != nil {
			return Contract{}, fmt.Errorf("saga create: insert milestones: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Contract{}, fmt.Errorf("saga create: commit: %w", err)
	}

	// Call Payments to hold funds (outside transaction — HTTP call)
	holdResp, err := s.payments.HoldFunds(ctx, HoldRequest{
		WalletID:   c.ClientWalletID,
		Amount:     c.Amount,
		ContractID: c.ID,
	})
	if err != nil {
		// Hold failed — mark contract cancelled
		slog.ErrorContext(ctx, "saga create: hold failed", "error", err, "contract_id", c.ID)
		cancelTx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			slog.ErrorContext(ctx, "saga create: compensation begin tx failed", "error", txErr, "contract_id", c.ID)
		} else {
			if err := s.contracts.UpdateStatus(ctx, cancelTx, c.ID, StatusPending, StatusCancelled); err != nil {
				slog.ErrorContext(ctx, "saga create: compensation update status failed", "error", err, "contract_id", c.ID)
			}
			payload, marshalErr := json.Marshal(c)
			if marshalErr != nil {
				slog.ErrorContext(ctx, "saga create: compensation marshal failed", "error", marshalErr, "contract_id", c.ID)
			} else if err := s.outbox.Insert(ctx, cancelTx, OutboxEntry{
				ID:            uuid.New().String(),
				AggregateType: "contract",
				AggregateID:   c.ID,
				EventType:     EventContractCancelled,
				Payload:       payload,
			}); err != nil {
				slog.ErrorContext(ctx, "saga create: compensation outbox insert failed", "error", err, "contract_id", c.ID)
			}
			if err := cancelTx.Commit(); err != nil {
				slog.ErrorContext(ctx, "saga create: compensation commit failed", "error", err, "contract_id", c.ID)
			}
		}
		c.Status = StatusCancelled
		return c, fmt.Errorf("saga create: hold funds: %w", err)
	}

	// Hold succeeded — update contract to HOLD_PENDING with hold_id
	updateTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: begin update tx: %w", err)
	}
	defer updateTx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, updateTx, c.ID, StatusPending, StatusHoldPending); err != nil {
		return Contract{}, fmt.Errorf("saga create: update to hold_pending: %w", err)
	}
	if err := s.contracts.SetHoldID(ctx, updateTx, c.ID, holdResp.ID); err != nil {
		return Contract{}, fmt.Errorf("saga create: set hold_id: %w", err)
	}

	payload, err := json.Marshal(c)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, updateTx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   c.ID,
		EventType:     EventContractCreated,
		Payload:       payload,
	}); err != nil {
		return Contract{}, fmt.Errorf("saga create: outbox insert: %w", err)
	}

	if err := updateTx.Commit(); err != nil {
		return Contract{}, fmt.Errorf("saga create: commit update: %w", err)
	}

	c.Status = StatusHoldPending
	c.HoldID = &holdResp.ID
	return c, nil
}

// HandlePaymentHeld advances contract from HOLD_PENDING → AWAITING_ACCEPT.
// Called by the NATS consumer when payment.held event is received.
func (s *SagaOrchestrator) HandlePaymentHeld(ctx context.Context, contractID string) error {
	return s.contracts.UpdateStatus(ctx, s.db, contractID, StatusHoldPending, StatusAwaitingAccept)
}

// HandlePaymentFailed marks contract as CANCELLED when hold fails.
// Called by the NATS consumer when payment.failed event is received.
func (s *SagaOrchestrator) HandlePaymentFailed(ctx context.Context, contractID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga payment failed: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusHoldPending, StatusCancelled); err != nil {
		return fmt.Errorf("saga payment failed: update status: %w", err)
	}

	c, err := s.contracts.GetByID(ctx, tx, contractID)
	if err != nil {
		return fmt.Errorf("saga payment failed: get contract: %w", err)
	}

	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("saga: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractCancelled,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga payment failed: outbox insert: %w", err)
	}

	return tx.Commit()
}

// AcceptContract transitions AWAITING_ACCEPT → ACTIVE.
func (s *SagaOrchestrator) AcceptContract(ctx context.Context, contractID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga accept: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusAwaitingAccept, StatusActive); err != nil {
		return fmt.Errorf("saga accept: update status: %w", err)
	}

	c, err := s.contracts.GetByID(ctx, tx, contractID)
	if err != nil {
		return fmt.Errorf("saga accept: get contract: %w", err)
	}

	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("saga: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractAccepted,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga accept: outbox insert: %w", err)
	}

	return tx.Commit()
}

// CompleteContract transitions ACTIVE → COMPLETING → calls transfer → COMPLETED.
// Also handles retry from COMPLETING (idempotent).
func (s *SagaOrchestrator) CompleteContract(ctx context.Context, contractID string) error {
	c, err := s.contracts.GetByID(ctx, s.db, contractID)
	if err != nil {
		return fmt.Errorf("saga complete: get contract: %w", err)
	}

	if c.Status == StatusActive {
		if err := s.contracts.UpdateStatus(ctx, s.db, contractID, StatusActive, StatusCompleting); err != nil {
			return fmt.Errorf("saga complete: update to completing: %w", err)
		}
		c.Status = StatusCompleting
	}

	if c.Status != StatusCompleting {
		return ErrStatusConflict
	}

	if c.HoldID == nil {
		return fmt.Errorf("saga complete: no hold_id on contract %s", contractID)
	}

	if err := s.payments.TransferFunds(ctx, *c.HoldID, c.FreelancerWalletID); err != nil {
		return fmt.Errorf("saga complete: transfer: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga complete: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusCompleting, StatusCompleted); err != nil {
		return fmt.Errorf("saga complete: update to completed: %w", err)
	}

	c.Status = StatusCompleted
	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("saga: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractCompleted,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga complete: outbox insert: %w", err)
	}

	return tx.Commit()
}

// CancelContract transitions AWAITING_ACCEPT → DECLINING → calls release → DECLINED.
// Also handles retry from DECLINING (idempotent).
func (s *SagaOrchestrator) CancelContract(ctx context.Context, contractID string) error {
	c, err := s.contracts.GetByID(ctx, s.db, contractID)
	if err != nil {
		return fmt.Errorf("saga cancel: get contract: %w", err)
	}

	if c.Status == StatusAwaitingAccept {
		if err := s.contracts.UpdateStatus(ctx, s.db, contractID, StatusAwaitingAccept, StatusDeclining); err != nil {
			return fmt.Errorf("saga cancel: update to declining: %w", err)
		}
		c.Status = StatusDeclining
	}

	if c.Status != StatusDeclining {
		return ErrStatusConflict
	}

	if c.HoldID == nil {
		return fmt.Errorf("saga cancel: no hold_id on contract %s", contractID)
	}

	if err := s.payments.ReleaseFunds(ctx, *c.HoldID); err != nil {
		return fmt.Errorf("saga cancel: release: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga cancel: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusDeclining, StatusDeclined); err != nil {
		return fmt.Errorf("saga cancel: update to declined: %w", err)
	}

	c.Status = StatusDeclined
	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("saga: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractDeclined,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga cancel: outbox insert: %w", err)
	}

	return tx.Commit()
}
