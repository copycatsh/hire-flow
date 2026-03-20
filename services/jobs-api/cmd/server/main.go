package main

import (
	"cmp"
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/copycatsh/hire-flow/pkg/telemetry"
	"github.com/copycatsh/hire-flow/services/jobs-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dbURL := cmp.Or(os.Getenv("DATABASE_URL"), "postgres://hire_flow:hire_flow_dev@localhost:5432/jobs_db?sslmode=disable")
	natsURL := cmp.Or(os.Getenv("NATS_URL"), "nats://localhost:4222")
	port := cmp.Or(os.Getenv("PORT"), ":8001")
	pollInterval, err := time.ParseDuration(cmp.Or(os.Getenv("OUTBOX_POLL_INTERVAL"), "1s"))
	if err != nil {
		slog.Error("invalid OUTBOX_POLL_INTERVAL", "error", err)
		os.Exit(1)
	}

	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, "jobs-api", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			slog.Error("telemetry shutdown", "error", err)
		}
	}()

	// Run migrations
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		slog.ErrorContext(ctx, "open sql db for migrations", "error", err)
		os.Exit(1)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		slog.ErrorContext(ctx, "goose set dialect", "error", err)
		os.Exit(1)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		slog.ErrorContext(ctx, "goose up", "error", err)
		os.Exit(1)
	}
	sqlDB.Close()

	// pgxpool
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.ErrorContext(ctx, "pgxpool new", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// NATS
	nc, err := NewNATSClient(natsURL)
	if err != nil {
		slog.ErrorContext(ctx, "nats connect", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.EnsureStream(ctx); err != nil {
		slog.ErrorContext(ctx, "nats ensure stream", "error", err)
		os.Exit(1)
	}

	// Stores
	jobStore := &PostgresJobStore{}
	profileStore := &PostgresProfileStore{}
	outboxStore := &outbox.PostgresStore{}

	// Echo
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = customErrorHandler
	e.Use(requestLogger)
	e.Use(otelecho.Middleware("jobs-api"))

	// Routes
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/metrics", echo.WrapHandler(telemetry.MetricsHandler()))

	jobHandler := &JobHandler{pool: pool, jobs: jobStore, outbox: outboxStore}
	jobHandler.RegisterRoutes(e.Group("/api/v1/jobs"))

	profileHandler := &ProfileHandler{pool: pool, profiles: profileStore, outbox: outboxStore}
	profileHandler.RegisterRoutes(e.Group("/api/v1/profiles"))

	// Outbox publisher
	publisher := outbox.NewPublisher(outboxStore, pool, nc, pollInterval)

	var wg sync.WaitGroup
	wg.Go(func() {
		publisher.Run(ctx)
	})

	// Start server
	go func() {
		slog.InfoContext(ctx, "starting jobs-api", "port", port)
		if err := e.Start(port); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed", "error", err)
			stop()
		}
	}()

	// Graceful shutdown
	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "echo shutdown", "error", err)
	}

	wg.Wait()
}
