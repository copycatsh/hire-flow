package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testUsers = []struct {
	UserID  uuid.UUID
	Name    string
	Balance int64
}{
	{uuid.MustParse("11111111-1111-1111-1111-111111111111"), "Alice (Client)", 100000},
	{uuid.MustParse("22222222-2222-2222-2222-222222222222"), "Bob (Client)", 50000},
	{uuid.MustParse("33333333-3333-3333-3333-333333333333"), "Carol (Freelancer)", 0},
	{uuid.MustParse("44444444-4444-4444-4444-444444444444"), "Dave (Freelancer)", 0},
	{uuid.MustParse("55555555-5555-5555-5555-555555555555"), "Eve (Admin)", 0},
}

func main() {
	dbURL := cmp.Or(os.Getenv("DATABASE_URL"), "postgres://hire_flow:hire_flow_dev@localhost:5433/payments_db?sslmode=disable")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("connect", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	for _, u := range testUsers {
		_, err := pool.Exec(ctx,
			`INSERT INTO wallets (user_id, balance, currency)
			 VALUES ($1, $2, 'USD')
			 ON CONFLICT (user_id) DO NOTHING`,
			u.UserID, u.Balance,
		)
		if err != nil {
			slog.Error("seed wallet", "user", u.Name, "error", err)
			os.Exit(1)
		}
		fmt.Printf("Seeded wallet: %s (user_id=%s, balance=$%.2f)\n", u.Name, u.UserID, float64(u.Balance)/100)
	}

	fmt.Println("Done.")
}
