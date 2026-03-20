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
	"github.com/copycatsh/hire-flow/services/payments/migrations"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dbURL := cmp.Or(os.Getenv("DATABASE_URL"), "postgres://hire_flow:hire_flow_dev@localhost:5433/payments_db?sslmode=disable")
	natsURL := cmp.Or(os.Getenv("NATS_URL"), "nats://localhost:4222")
	port := cmp.Or(os.Getenv("PORT"), ":8004")
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

	shutdownTelemetry, err := telemetry.Init(ctx, "payments", otelEndpoint)
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
	outboxStore := &outbox.PostgresStore{}

	// Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(otelgin.Middleware("payments"))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(telemetry.MetricsHandler()))

	handler := &PaymentHandler{
		pool:         pool,
		wallets:      &PostgresWalletStore{},
		holds:        &PostgresHoldStore{},
		transactions: &PostgresTransactionStore{},
		outbox:       outboxStore,
	}
	handler.RegisterRoutes(r)

	// Outbox publisher
	publisher := outbox.NewPublisher(outboxStore, pool, nc, pollInterval)

	var wg sync.WaitGroup
	wg.Go(func() {
		publisher.Run(ctx)
	})

	// Start server
	srv := &http.Server{Addr: port, Handler: r}
	go func() {
		slog.InfoContext(ctx, "starting payments", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed", "error", err)
			stop()
		}
	}()

	// Graceful shutdown
	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "server shutdown", "error", err)
	}

	wg.Wait()
}
