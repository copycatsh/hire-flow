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

	"github.com/copycatsh/hire-flow/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	port := cmp.Or(os.Getenv("PORT"), ":8011")
	if port[0] != ':' {
		port = ":" + port
	}
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

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

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.Handle("GET /metrics", telemetry.MetricsHandler())

	srv := &http.Server{
		Addr:    port,
		Handler: otelhttp.NewHandler(mux, "bff-freelancer"),
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
