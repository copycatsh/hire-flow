package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ContractHandler struct {
	saga       *SagaOrchestrator
	contracts  ContractStore
	milestones MilestoneStore
	db         *sql.DB
}

func (h *ContractHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/contracts", func(r chi.Router) {
		r.Post("/", h.CreateContract)
		r.Put("/{id}/accept", h.AcceptContract)
		r.Put("/{id}/complete", h.CompleteContract)
		r.Put("/{id}/cancel", h.CancelContract)
		r.Get("/{id}", h.GetContract)
	})
}

func (h *ContractHandler) CreateContract(w http.ResponseWriter, r *http.Request) {
	var req CreateContractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ClientID == "" || req.FreelancerID == "" || req.Title == "" || req.Amount <= 0 || req.ClientWalletID == "" || req.FreelancerWalletID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id, freelancer_id, title, amount, client_wallet_id, freelancer_wallet_id are required"})
		return
	}

	c := Contract{
		ClientID:           req.ClientID,
		FreelancerID:       req.FreelancerID,
		Title:              req.Title,
		Description:        req.Description,
		Amount:             req.Amount,
		ClientWalletID:     req.ClientWalletID,
		FreelancerWalletID: req.FreelancerWalletID,
	}

	var milestones []Milestone
	for _, ms := range req.Milestones {
		milestones = append(milestones, Milestone{
			Title:       ms.Title,
			Description: ms.Description,
			Amount:      ms.Amount,
			Position:    ms.Position,
		})
	}

	result, err := h.saga.CreateContract(r.Context(), c, milestones)
	if err != nil {
		slog.Error("create contract", "error", err)
		if result.Status == StatusCancelled {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "payment hold failed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *ContractHandler) AcceptContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("accept contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.AcceptContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be accepted in current status"})
			return
		}
		slog.Error("accept contract", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	c, err := h.contracts.GetByID(r.Context(), h.db, id)
	if err != nil {
		slog.Error("accept contract: refetch", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) CompleteContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("complete contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.CompleteContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be completed in current status"})
			return
		}
		slog.Error("complete contract", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment service unavailable, retry later"})
		return
	}

	c, err := h.contracts.GetByID(r.Context(), h.db, id)
	if err != nil {
		slog.Error("complete contract: refetch", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) CancelContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("cancel contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.CancelContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be cancelled in current status"})
			return
		}
		slog.Error("cancel contract", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment service unavailable, retry later"})
		return
	}

	c, err := h.contracts.GetByID(r.Context(), h.db, id)
	if err != nil {
		slog.Error("cancel contract: refetch", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) GetContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	c, err := h.contracts.GetByID(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("get contract", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, c)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
	}
}
