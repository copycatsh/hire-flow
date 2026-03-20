package main

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/copycatsh/hire-flow/pkg/telemetry"
	"github.com/copycatsh/hire-flow/services/contracts/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := cmp.Or(os.Getenv("DATABASE_URL"), "hire_flow:hire_flow_dev@tcp(localhost:3306)/contracts_db?parseTime=true")
	natsURL := cmp.Or(os.Getenv("NATS_URL"), "nats://localhost:4222")
	paymentsURL := cmp.Or(os.Getenv("PAYMENTS_URL"), "http://localhost:8004")
	port := cmp.Or(os.Getenv("PORT"), ":8003")
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

	shutdownTelemetry, err := telemetry.Init(ctx, "contracts", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			slog.Error("telemetry shutdown", "error", err)
		}
	}()

	// MySQL
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.ErrorContext(ctx, "open mysql", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Run migrations
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("mysql"); err != nil {
		slog.ErrorContext(ctx, "goose set dialect", "error", err)
		os.Exit(1)
	}
	if err := goose.Up(db, "."); err != nil {
		slog.ErrorContext(ctx, "goose up", "error", err)
		os.Exit(1)
	}

	// NATS
	nc, err := NewNATSClient(natsURL)
	if err != nil {
		slog.ErrorContext(ctx, "nats connect", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.EnsureStream(ctx); err != nil {
		slog.ErrorContext(ctx, "nats ensure contracts stream", "error", err)
		os.Exit(1)
	}

	paymentsConsumer, err := nc.EnsurePaymentsConsumer(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "nats ensure payments consumer", "error", err)
		os.Exit(1)
	}

	// Stores
	contractStore := &MySQLContractStore{}
	milestoneStore := &MySQLMilestoneStore{}
	outboxStore := &MySQLOutboxStore{}

	// Saga + handler
	paymentsClient := NewPaymentsClient(paymentsURL)
	saga := NewSagaOrchestrator(db, contractStore, milestoneStore, outboxStore, paymentsClient)

	handler := &ContractHandler{
		saga:       saga,
		contracts:  contractStore,
		milestones: milestoneStore,
		db:         db,
	}

	// Chi router
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	r.Handle("/metrics", telemetry.MetricsHandler())
	handler.RegisterRoutes(r)

	// Outbox publisher
	publisher := NewOutboxPublisher(outboxStore, db, nc, pollInterval)

	var wg sync.WaitGroup

	// Start outbox publisher
	wg.Go(func() {
		publisher.Run(ctx)
	})

	// Start NATS consumer
	wg.Go(func() {
		runPaymentsConsumer(ctx, paymentsConsumer, saga)
	})

	// Start HTTP server
	srv := &http.Server{Addr: port, Handler: otelhttp.NewHandler(r, "contracts")}
	go func() {
		slog.InfoContext(ctx, "starting contracts", "port", port)
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

// runPaymentsConsumer pulls payment events from NATS and dispatches to saga.
func runPaymentsConsumer(ctx context.Context, consumer jetstream.Consumer, saga *SagaOrchestrator) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.ErrorContext(ctx, "nats fetch", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			subject := msg.Subject()
			slog.InfoContext(ctx, "received payment event", "subject", subject)

			var payload struct {
				ContractID string `json:"contract_id"`
			}
			if err := json.Unmarshal(msg.Data(), &payload); err != nil {
				slog.ErrorContext(ctx, "nats unmarshal", "error", err, "subject", subject)
				msg.Nak()
				continue
			}

			if payload.ContractID == "" {
				slog.ErrorContext(ctx, "nats event missing contract_id", "subject", subject, "data", string(msg.Data()))
				msg.Term()
				continue
			}

			var handleErr error
			switch subject {
			case "payments.payment.held":
				handleErr = saga.HandlePaymentHeld(ctx, payload.ContractID)
			case "payments.payment.failed":
				handleErr = saga.HandlePaymentFailed(ctx, payload.ContractID)
			default:
				slog.InfoContext(ctx, "ignoring payment event", "subject", subject)
			}

			if handleErr != nil {
				slog.ErrorContext(ctx, "saga handle event", "error", handleErr, "subject", subject, "contract_id", payload.ContractID)
				msg.Nak()
				continue
			}

			msg.Ack()
		}
	}
}
