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
	"github.com/copycatsh/hire-flow/services/payments/migrations"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run migrations
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		slog.Error("open sql db for migrations", "error", err)
		os.Exit(1)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("goose set dialect", "error", err)
		os.Exit(1)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		slog.Error("goose up", "error", err)
		os.Exit(1)
	}
	sqlDB.Close()

	// pgxpool
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("pgxpool new", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// NATS
	nc, err := NewNATSClient(natsURL)
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.EnsureStream(ctx); err != nil {
		slog.Error("nats ensure stream", "error", err)
		os.Exit(1)
	}

	// Stores
	outboxStore := &outbox.PostgresStore{}

	// Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

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
		slog.Info("starting payments", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			stop()
		}
	}()

	// Graceful shutdown
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown", "error", err)
	}

	wg.Wait()
}
