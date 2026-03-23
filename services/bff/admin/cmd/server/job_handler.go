package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type JobHandler struct {
	jobs *bff.ServiceClient
}

func (h *JobHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/jobs", h.List)
	mux.HandleFunc("GET /api/v1/jobs/{id}", h.GetByID)
}

func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/jobs"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.jobs.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *JobHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.jobs.Forward(r.Context(), w, http.MethodGet, "/api/v1/jobs/"+id, nil)
}
