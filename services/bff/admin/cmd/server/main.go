package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("encoding health response", "error", err)
		}
	})

	port := ":8012"
	slog.Info("starting bff-admin", "port", port)
	if err := http.ListenAndServe(port, mux); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
