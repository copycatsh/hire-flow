package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type MatchHandler struct {
	matching *bff.ServiceClient
}

func (h *MatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/{id}/matches", h.FindMatches)
}

func (h *MatchHandler) FindMatches(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	path := "/api/v1/match/job/" + id
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.matching.Forward(r.Context(), w, http.MethodPost, path, r.Body)
}
