package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type ContractHandler struct {
	contracts *bff.ServiceClient
}

func (h *ContractHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/contracts", h.List)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetByID)
}

func (h *ContractHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/contracts"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.contracts.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *ContractHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodGet, "/api/v1/contracts/"+id, nil)
}
