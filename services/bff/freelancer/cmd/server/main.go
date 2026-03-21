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

	"github.com/copycatsh/hire-flow/pkg/bff"
	"github.com/copycatsh/hire-flow/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	port := cmp.Or(os.Getenv("PORT"), "8011")
	if port[0] != ':' {
		port = ":" + port
	}
	jwtSecret := cmp.Or(os.Getenv("JWT_SECRET"), "dev-secret-change-in-production")
	matchingURL := cmp.Or(os.Getenv("MATCHING_URL"), "http://ai-matching:8002")
	contractsURL := cmp.Or(os.Getenv("CONTRACTS_URL"), "http://contracts:8003")
	paymentsURL := cmp.Or(os.Getenv("PAYMENTS_URL"), "http://payments:8004")
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")
	cookieSecure := os.Getenv("COOKIE_SECURE") != "false"

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, "bff-freelancer", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			slog.Error("telemetry shutdown", "error", err)
		}
	}()

	auth := &bff.AuthConfig{
		Secret:          []byte(jwtSecret),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		CookieSecure:    cookieSecure,
	}

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	matchingClient := &bff.ServiceClient{BaseURL: matchingURL, HTTP: httpClient, Name: "ai-matching"}
	contractsClient := &bff.ServiceClient{BaseURL: contractsURL, HTTP: httpClient, Name: "contracts"}
	paymentsClient := &bff.ServiceClient{BaseURL: paymentsURL, HTTP: httpClient, Name: "payments"}

	rateLimiter := bff.NewRateLimiter(100, 20)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.Handle("GET /metrics", telemetry.MetricsHandler())

	authHandler := &bff.AuthHandler{Auth: auth}
	authHandler.RegisterRoutes(mux)

	apiMux := http.NewServeMux()

	matchHandler := &MatchHandler{matching: matchingClient}
	matchHandler.RegisterRoutes(apiMux)

	contractHandler := &ContractHandler{contracts: contractsClient}
	contractHandler.RegisterRoutes(apiMux)

	paymentHandler := &PaymentHandler{payments: paymentsClient}
	paymentHandler.RegisterRoutes(apiMux)

	protected := auth.JWTMiddleware(rateLimiter.Middleware(apiMux))
	mux.Handle("/api/", protected)

	handler := otelhttp.NewHandler(bff.RequestLogger(mux), "bff-freelancer")

	srv := &http.Server{
		Addr:         port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.InfoContext(ctx, "starting bff-freelancer", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down bff-freelancer")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "shutdown error", "error", err)
	}
}