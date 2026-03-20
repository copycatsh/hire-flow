package main

import (
	"net/http"
	"net/url"
)

type PaymentHandler struct {
	payments *ServiceClient
}

func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallet", h.GetBalance)
}

func (h *PaymentHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, "/api/v1/payments/wallet/"+url.PathEscape(userID), nil)
}