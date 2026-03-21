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
	mux.HandleFunc("POST /api/v1/matches", h.FindJobMatches)
}

func (h *MatchHandler) FindJobMatches(w http.ResponseWriter, r *http.Request) {
	// Use profile_id from query if provided, otherwise fall back to authenticated user_id
	profileID := r.URL.Query().Get("profile_id")
	if profileID == "" {
		profileID = bff.UserIDFrom(r.Context())
	}
	if profileID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	path := "/api/v1/match/profile/" + url.PathEscape(profileID)
	if topK := r.URL.Query().Get("top_k"); topK != "" {
		path += "?top_k=" + url.QueryEscape(topK)
	}
	h.matching.Forward(r.Context(), w, http.MethodPost, path, r.Body)
}
