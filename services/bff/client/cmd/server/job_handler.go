package main

import (
	"net/http"
	"net/url"
)

type JobHandler struct {
	jobs *ServiceClient
}

func (h *JobHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs", h.Create)
	mux.HandleFunc("GET /api/v1/jobs", h.List)
	mux.HandleFunc("GET /api/v1/jobs/{id}", h.GetByID)
}

func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.jobs.Forward(r.Context(), w, http.MethodPost, "/api/v1/jobs", r.Body)
}

func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/jobs"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.jobs.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *JobHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	h.jobs.Forward(r.Context(), w, http.MethodGet, "/api/v1/jobs/"+id, nil)
}