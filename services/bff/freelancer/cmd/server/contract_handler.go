package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type ContractHandler struct {
	contracts *bff.ServiceClient
}

func (h *ContractHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/contracts", h.List)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/contracts/{id}/accept", h.Accept)
}

func (h *ContractHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := bff.UserIDFrom(r.Context())
	if userID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	path := "/api/v1/contracts?freelancer_id=" + url.QueryEscape(userID)
	h.contracts.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *ContractHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	h.contracts.Forward(r.Context(), w, http.MethodGet, "/api/v1/contracts/"+id, nil)
}

func (h *ContractHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	h.contracts.Forward(r.Context(), w, http.MethodPut, "/api/v1/contracts/"+id+"/accept", r.Body)
}
