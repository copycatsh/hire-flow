package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type PaymentHandler struct {
	payments *bff.ServiceClient
}

func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallet", h.GetBalance)
}

func (h *PaymentHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID := bff.UserIDFrom(r.Context())
	if userID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, "/api/v1/payments/wallet/"+url.PathEscape(userID), nil)
}
