package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentHandler struct {
	pool         *pgxpool.Pool
	wallets      WalletStore
	holds        HoldStore
	transactions TransactionStore
	outbox       outbox.Store
}

func (h *PaymentHandler) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/api/v1/payments")
	g.POST("/hold", h.HoldFunds)
	g.POST("/release", h.ReleaseFunds)
	g.POST("/transfer", h.TransferFunds)
	g.GET("/wallet/:user_id", h.GetWallet)
}

func (h *PaymentHandler) HoldFunds(c *gin.Context) {
	var req HoldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.WalletID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wallet_id is required"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be positive"})
		return
	}
	if req.ContractID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "contract_id is required"})
		return
	}

	ctx := c.Request.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("hold: begin tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer tx.Rollback(ctx)

	wallet, err := h.wallets.GetByIDForUpdate(ctx, tx, req.WalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
			return
		}
		slog.Error("hold: get wallet", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if wallet.AvailableBalance < req.Amount {
		failedTx, txErr := h.transactions.Create(ctx, tx, Transaction{
			WalletID: wallet.ID,
			Amount:   -req.Amount,
			Type:     "hold_failed",
		})
		if txErr != nil {
			slog.Error("hold: create failed tx", "error", txErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		payload, marshalErr := json.Marshal(map[string]any{
			"wallet_id":   wallet.ID,
			"amount":      req.Amount,
			"contract_id": req.ContractID,
			"reason":      "insufficient funds",
		})
		if marshalErr != nil {
			slog.Error("hold: marshal failed event payload", "error", marshalErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if err := h.outbox.Insert(ctx, tx, outbox.Entry{
			AggregateType: "payment",
			AggregateID:   failedTx.ID,
			EventType:     EventPaymentFailed,
			Payload:       payload,
		}); err != nil {
			slog.Error("hold: insert failed outbox", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("hold: commit failed tx", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "insufficient funds"})
		return
	}

	hold, err := h.holds.Create(ctx, tx, wallet.ID, req.Amount, req.ContractID)
	if err != nil {
		slog.Error("hold: create hold", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	_, err = h.transactions.Create(ctx, tx, Transaction{
		WalletID: wallet.ID,
		Amount:   -req.Amount,
		Type:     "hold",
		HoldID:   &hold.ID,
	})
	if err != nil {
		slog.Error("hold: create tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	payload, err := json.Marshal(hold)
	if err != nil {
		slog.Error("hold: marshal payload", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	err = h.outbox.Insert(ctx, tx, outbox.Entry{
		AggregateType: "payment",
		AggregateID:   hold.ID,
		EventType:     EventPaymentHeld,
		Payload:       payload,
	})
	if err != nil {
		slog.Error("hold: insert outbox", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("hold: commit", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, hold)
}

func (h *PaymentHandler) ReleaseFunds(c *gin.Context) {
	var req ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.HoldID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hold_id is required"})
		return
	}

	ctx := c.Request.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("release: begin tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer tx.Rollback(ctx)

	hold, err := h.holds.GetByIDForUpdate(ctx, tx, req.HoldID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "hold not found"})
			return
		}
		slog.Error("release: get hold", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if hold.Status != "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "hold is not active"})
		return
	}

	if err := h.holds.UpdateStatus(ctx, tx, hold.ID, "released"); err != nil {
		slog.Error("release: update status", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	hold.Status = "released"

	_, err = h.transactions.Create(ctx, tx, Transaction{
		WalletID: hold.WalletID,
		Amount:   hold.Amount,
		Type:     "release",
		HoldID:   &hold.ID,
	})
	if err != nil {
		slog.Error("release: create tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	payload, err := json.Marshal(hold)
	if err != nil {
		slog.Error("release: marshal payload", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	err = h.outbox.Insert(ctx, tx, outbox.Entry{
		AggregateType: "payment",
		AggregateID:   hold.ID,
		EventType:     EventPaymentReleased,
		Payload:       payload,
	})
	if err != nil {
		slog.Error("release: insert outbox", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("release: commit", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, hold)
}

func (h *PaymentHandler) TransferFunds(c *gin.Context) {
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.HoldID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hold_id is required"})
		return
	}
	if req.RecipientWalletID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recipient_wallet_id is required"})
		return
	}

	ctx := c.Request.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		slog.Error("transfer: begin tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	defer tx.Rollback(ctx)

	hold, err := h.holds.GetByIDForUpdate(ctx, tx, req.HoldID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "hold not found"})
			return
		}
		slog.Error("transfer: get hold", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if hold.Status != "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "hold is not active"})
		return
	}

	if hold.WalletID == req.RecipientWalletID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot transfer to source wallet"})
		return
	}

	source, err := h.wallets.GetByIDForUpdate(ctx, tx, hold.WalletID)
	if err != nil {
		slog.Error("transfer: get source wallet", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if source.Balance < hold.Amount {
		slog.Warn("transfer: balance invariant violation",
			"wallet_id", source.ID, "balance", source.Balance, "hold_amount", hold.Amount)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	recipient, err := h.wallets.GetByIDForUpdate(ctx, tx, req.RecipientWalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "recipient wallet not found"})
			return
		}
		slog.Error("transfer: get recipient wallet", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := h.wallets.UpdateBalance(ctx, tx, source.ID, source.Balance-hold.Amount); err != nil {
		slog.Error("transfer: debit source", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := h.wallets.UpdateBalance(ctx, tx, recipient.ID, recipient.Balance+hold.Amount); err != nil {
		slog.Error("transfer: credit recipient", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := h.holds.UpdateStatus(ctx, tx, hold.ID, "transferred"); err != nil {
		slog.Error("transfer: update hold status", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	_, err = h.transactions.Create(ctx, tx, Transaction{
		WalletID: source.ID,
		Amount:   -hold.Amount,
		Type:     "transfer_debit",
		HoldID:   &hold.ID,
	})
	if err != nil {
		slog.Error("transfer: create debit tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	_, err = h.transactions.Create(ctx, tx, Transaction{
		WalletID: recipient.ID,
		Amount:   hold.Amount,
		Type:     "transfer_credit",
		HoldID:   &hold.ID,
	})
	if err != nil {
		slog.Error("transfer: create credit tx", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	payload, err := json.Marshal(map[string]any{
		"hold_id":              hold.ID,
		"source_wallet_id":    source.ID,
		"recipient_wallet_id": recipient.ID,
		"amount":              hold.Amount,
		"contract_id":         hold.ContractID,
	})
	if err != nil {
		slog.Error("transfer: marshal payload", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	err = h.outbox.Insert(ctx, tx, outbox.Entry{
		AggregateType: "payment",
		AggregateID:   hold.ID,
		EventType:     EventPaymentTransferred,
		Payload:       payload,
	})
	if err != nil {
		slog.Error("transfer: insert outbox", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("transfer: commit", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"hold_id":              hold.ID,
		"source_wallet_id":    source.ID,
		"recipient_wallet_id": recipient.ID,
		"amount":              hold.Amount,
		"status":              "transferred",
	})
}

func (h *PaymentHandler) GetWallet(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	wallet, err := h.wallets.GetByUserID(c.Request.Context(), h.pool, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
			return
		}
		slog.Error("get wallet", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, wallet)
}
