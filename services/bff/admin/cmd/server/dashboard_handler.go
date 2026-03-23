package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type DashboardHandler struct {
	jobs      *bff.ServiceClient
	contracts *bff.ServiceClient
	payments  *bff.ServiceClient
}

func (h *DashboardHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dashboard/stats", h.Stats)
}

type serviceStats struct {
	Total int    `json:"total"`
	Error string `json:"error,omitzero"`
}

type dashboardResponse struct {
	Jobs      serviceStats `json:"jobs"`
	Contracts serviceStats `json:"contracts"`
	Wallets   serviceStats `json:"wallets"`
}

func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var resp dashboardResponse
	var mu sync.Mutex
	var wg sync.WaitGroup

	type listResp struct {
		Total int `json:"total"`
	}

	fetch := func(client *bff.ServiceClient, path string, target *serviceStats) {
		defer wg.Done()
		var result listResp
		err := client.Do(ctx, http.MethodGet, path, nil, &result)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			target.Error = client.Name + " unavailable"
			return
		}
		target.Total = result.Total
	}

	wg.Add(3)
	go fetch(h.jobs, "/api/v1/jobs?limit=1", &resp.Jobs)
	go fetch(h.contracts, "/api/v1/contracts?limit=1", &resp.Contracts)
	go fetch(h.payments, "/api/v1/payments/wallets?limit=1", &resp.Wallets)
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
