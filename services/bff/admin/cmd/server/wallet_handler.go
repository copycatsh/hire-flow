package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type WalletHandler struct {
	payments *bff.ServiceClient
}

func (h *WalletHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallets", h.List)
}

func (h *WalletHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/payments/wallets"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, path, nil)
}
