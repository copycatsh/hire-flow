package main

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := cmp.Or(os.Getenv("PORT"), "8010")
	if port[0] != ':' {
		port = ":" + port
	}
	jwtSecret := cmp.Or(os.Getenv("JWT_SECRET"), "dev-secret-change-in-production")
	jobsURL := cmp.Or(os.Getenv("JOBS_URL"), "http://jobs-api:8001")
	matchingURL := cmp.Or(os.Getenv("MATCHING_URL"), "http://ai-matching:8002")
	contractsURL := cmp.Or(os.Getenv("CONTRACTS_URL"), "http://contracts:8003")
	paymentsURL := cmp.Or(os.Getenv("PAYMENTS_URL"), "http://payments:8004")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	auth := &AuthConfig{
		Secret:          []byte(jwtSecret),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	jobsClient := &ServiceClient{BaseURL: jobsURL, HTTP: httpClient, Name: "jobs-api"}
	matchingClient := &ServiceClient{BaseURL: matchingURL, HTTP: httpClient, Name: "ai-matching"}
	contractsClient := &ServiceClient{BaseURL: contractsURL, HTTP: httpClient, Name: "contracts"}
	paymentsClient := &ServiceClient{BaseURL: paymentsURL, HTTP: httpClient, Name: "payments"}

	rateLimiter := NewRateLimiter(100, 20)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	authHandler := &AuthHandler{auth: auth}
	authHandler.RegisterRoutes(mux)

	apiMux := http.NewServeMux()

	jobHandler := &JobHandler{jobs: jobsClient}
	jobHandler.RegisterRoutes(apiMux)

	matchHandler := &MatchHandler{matching: matchingClient}
	matchHandler.RegisterRoutes(apiMux)

	contractHandler := &ContractHandler{contracts: contractsClient}
	contractHandler.RegisterRoutes(apiMux)

	paymentHandler := &PaymentHandler{payments: paymentsClient}
	paymentHandler.RegisterRoutes(apiMux)

	protected := auth.JWTMiddleware(rateLimiter.Middleware(apiMux))
	mux.Handle("/api/", protected)

	handler := RequestLogger(mux)

	srv := &http.Server{
		Addr:         port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting bff-client", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down bff-client")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}