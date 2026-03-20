# M4 — Contracts Service + Saga Orchestrator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the Contracts service (Go/Chi + MySQL) with saga orchestration for the contract lifecycle: propose → hold payment → accept → complete → pay, with compensation flows for decline/cancel.

**Architecture:** Contracts is the saga orchestrator. It stores contract state in MySQL, calls Payments via HTTP for commands (hold/release/transfer), listens to NATS for payment events (payment.held, payment.failed) to advance saga state, and publishes its own events (contract.created, contract.accepted, etc.) via a MySQL-local outbox → NATS. The saga state is tracked as a status column on the contracts table (no separate saga table). Milestones exist in the schema but completion is contract-level (all-or-nothing) for M4.

**Tech Stack:** Go 1.25, Chi v5, database/sql + go-sql-driver/mysql, goose (migrations), NATS JetStream, testcontainers-go (MySQL + NATS modules), testify

**Eng Review Decisions (2026-03-19):**

| # | Decision | Choice |
|---|---|---|
| 1 | Outbox DB | MySQL outbox in contracts service (local store + publisher) |
| 2 | Saga communication | HTTP for commands to Payments, NATS for events |
| 3 | Saga state storage | Status column on contracts table |
| 4 | NATS consumer | Durable pull consumer `contracts-saga` on PAYMENTS stream |
| 5 | Outbox publisher | Local MySQL publisher in contracts |
| 6 | Milestones | Contract-level completion, milestones in schema only |
| 7 | Saga file structure | Single saga.go |
| 8 | Test infrastructure | Testcontainers (MySQL + NATS) + mock Payments HTTP |
| 9 | Saga failure handling | Return error + idempotent retry (COMPLETING/DECLINING retriable) |

**Merge criteria:** Full contract flow works: propose → accept → complete → pay. Cancel triggers compensation.

---

## Task 1: MySQL migrations + goose embed

**Why:** Schema first. Everything depends on the database tables.

**Files:**
- Create: `services/contracts/migrations/embed.go`
- Create: `services/contracts/migrations/001_create_contracts.sql`
- Create: `services/contracts/migrations/002_create_milestones.sql`
- Create: `services/contracts/migrations/003_create_outbox.sql`

**Step 1: Create migrations directory and embed file**

```go
// services/contracts/migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

**Step 2: Create contracts table migration**

```sql
-- services/contracts/migrations/001_create_contracts.sql

-- +goose Up
CREATE TABLE contracts (
    id                CHAR(36) PRIMARY KEY,
    client_id         CHAR(36) NOT NULL,
    freelancer_id     CHAR(36) NOT NULL,
    title             VARCHAR(500) NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    amount            BIGINT NOT NULL CHECK (amount > 0),
    currency          VARCHAR(3) NOT NULL DEFAULT 'USD',
    status            VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    client_wallet_id  CHAR(36) NOT NULL,
    freelancer_wallet_id CHAR(36) NOT NULL,
    hold_id           CHAR(36),
    created_at        TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at        TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    INDEX idx_contracts_client (client_id),
    INDEX idx_contracts_freelancer (freelancer_id),
    INDEX idx_contracts_status (status)
);

-- +goose Down
DROP TABLE IF EXISTS contracts;
```

**Step 3: Create milestones table migration**

```sql
-- services/contracts/migrations/002_create_milestones.sql

-- +goose Up
CREATE TABLE milestones (
    id          CHAR(36) PRIMARY KEY,
    contract_id CHAR(36) NOT NULL,
    title       VARCHAR(500) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount      BIGINT NOT NULL CHECK (amount > 0),
    position    INT NOT NULL DEFAULT 0,
    status      VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at  TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at  TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    FOREIGN KEY (contract_id) REFERENCES contracts(id),
    INDEX idx_milestones_contract (contract_id)
);

-- +goose Down
DROP TABLE IF EXISTS milestones;
```

**Step 4: Create outbox table migration**

```sql
-- services/contracts/migrations/003_create_outbox.sql

-- +goose Up
CREATE TABLE outbox (
    id             CHAR(36) PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id   CHAR(36) NOT NULL,
    event_type     VARCHAR(100) NOT NULL,
    payload        JSON NOT NULL,
    created_at     TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    published_at   TIMESTAMP(6) NULL DEFAULT NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE (published_at IS NULL);

-- +goose Down
DROP TABLE IF EXISTS outbox;
```

**Step 5: Commit**

```bash
git add services/contracts/migrations/
git commit -m "feat(m4): add MySQL migrations — contracts, milestones, outbox"
```

---

## Task 2: Domain types + store interfaces

**Why:** Define the core domain types and interfaces. Everything else imports these.

**Files:**
- Create: `services/contracts/cmd/server/contract.go`
- Create: `services/contracts/cmd/server/milestone.go`
- Create: `services/contracts/cmd/server/events.go`
- Create: `services/contracts/cmd/server/db.go`

**Step 1: Create DBTX interface for database/sql**

```go
// services/contracts/cmd/server/db.go
package main

import (
	"context"
	"database/sql"
)

// DBTX abstracts *sql.DB and *sql.Tx for transaction polymorphism.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

**Step 2: Create contract domain type + store interface**

```go
// services/contracts/cmd/server/contract.go
package main

import (
	"context"
	"time"
)

// Contract status constants — saga states embedded in contract lifecycle.
//
//	                    ┌──────────┐
//	       POST /contracts        │
//	                    ▼         │
//	               ┌─────────┐   │
//	               │ PENDING  │   │
//	               └────┬─────┘   │
//	                    │ HTTP: POST /payments/hold
//	                    ▼
//	            ┌──────────────┐
//	            │ HOLD_PENDING │
//	            └──────┬───────┘
//	        ┌──────────┼──────────┐
//	  payment.failed   │   payment.held
//	        ▼          │          ▼
//	  ┌───────────┐    │   ┌────────────────┐
//	  │ CANCELLED │    │   │ AWAITING_ACCEPT │
//	  └───────────┘    │   └────┬────────────┘
//	              ┌────┴────┐   │
//	        PUT /accept  PUT /cancel
//	              ▼         ▼
//	        ┌────────┐  ┌───────────┐
//	        │ ACTIVE │  │ DECLINING │─── release ──▶ DECLINED
//	        └───┬────┘  └───────────┘
//	     PUT /complete
//	            ▼
//	     ┌────────────┐
//	     │ COMPLETING │─── transfer ──▶ COMPLETED
//	     └────────────┘
const (
	StatusPending        = "PENDING"
	StatusHoldPending    = "HOLD_PENDING"
	StatusAwaitingAccept = "AWAITING_ACCEPT"
	StatusActive         = "ACTIVE"
	StatusCompleting     = "COMPLETING"
	StatusCompleted      = "COMPLETED"
	StatusDeclining      = "DECLINING"
	StatusDeclined       = "DECLINED"
	StatusCancelled      = "CANCELLED"
)

type Contract struct {
	ID                 string    `json:"id"`
	ClientID           string    `json:"client_id"`
	FreelancerID       string    `json:"freelancer_id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Amount             int64     `json:"amount"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	ClientWalletID     string    `json:"client_wallet_id"`
	FreelancerWalletID string    `json:"freelancer_wallet_id"`
	HoldID             *string   `json:"hold_id,omitzero"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CreateContractRequest struct {
	ClientID           string          `json:"client_id"`
	FreelancerID       string          `json:"freelancer_id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Amount             int64           `json:"amount"`
	ClientWalletID     string          `json:"client_wallet_id"`
	FreelancerWalletID string          `json:"freelancer_wallet_id"`
	Milestones         []MilestoneSpec `json:"milestones"`
}

type MilestoneSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"`
	Position    int    `json:"position"`
}

type ContractStore interface {
	Create(ctx context.Context, db DBTX, c Contract) error
	GetByID(ctx context.Context, db DBTX, id string) (Contract, error)
	UpdateStatus(ctx context.Context, db DBTX, id string, from string, to string) error
	SetHoldID(ctx context.Context, db DBTX, id string, holdID string) error
}
```

**Step 3: Create milestone domain type + store interface**

```go
// services/contracts/cmd/server/milestone.go
package main

import (
	"context"
	"time"
)

type Milestone struct {
	ID          string    `json:"id"`
	ContractID  string    `json:"contract_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Position    int       `json:"position"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MilestoneStore interface {
	CreateBatch(ctx context.Context, db DBTX, milestones []Milestone) error
	ListByContract(ctx context.Context, db DBTX, contractID string) ([]Milestone, error)
}
```

**Step 4: Create event constants**

```go
// services/contracts/cmd/server/events.go
package main

const (
	EventContractCreated   = "contracts.contract.created"
	EventContractAccepted  = "contracts.contract.accepted"
	EventContractCompleted = "contracts.contract.completed"
	EventContractCancelled = "contracts.contract.cancelled"
	EventContractDeclined  = "contracts.contract.declined"
)
```

**Step 5: Commit**

```bash
git add services/contracts/cmd/server/contract.go services/contracts/cmd/server/milestone.go services/contracts/cmd/server/events.go services/contracts/cmd/server/db.go
git commit -m "feat(m4): add contract/milestone domain types, store interfaces, events"
```

---

## Task 3: MySQL store implementations

**Why:** Implement the store interfaces against MySQL. Required by handlers and saga.

**Files:**
- Create: `services/contracts/cmd/server/contract_store.go`
- Create: `services/contracts/cmd/server/milestone_store.go`

**Step 1: Write contract store tests**

Create `services/contracts/cmd/server/contract_store_test.go`:

```go
package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLContractStore_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Build API",
		Description:        "REST API development",
		Amount:             50000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}

	require.NoError(t, store.Create(ctx, db, c))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, c.Title, got.Title)
	assert.Equal(t, c.Amount, got.Amount)
	assert.Equal(t, StatusPending, got.Status)
	assert.Nil(t, got.HoldID)
}

func TestMySQLContractStore_UpdateStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test Contract",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, store.Create(ctx, db, c))

	// Valid transition
	require.NoError(t, store.UpdateStatus(ctx, db, c.ID, StatusPending, StatusHoldPending))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusHoldPending, got.Status)

	// Wrong "from" status → error
	err = store.UpdateStatus(ctx, db, c.ID, StatusPending, StatusActive)
	assert.Error(t, err)
}

func TestMySQLContractStore_SetHoldID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	c := Contract{
		ID:                 uuid.New().String(),
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test Contract",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, store.Create(ctx, db, c))

	holdID := uuid.New().String()
	require.NoError(t, store.SetHoldID(ctx, db, c.ID, holdID))

	got, err := store.GetByID(ctx, db, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got.HoldID)
	assert.Equal(t, holdID, *got.HoldID)
}

func TestMySQLContractStore_GetByID_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLContractStore{}

	_, err := store.GetByID(ctx, db, uuid.New().String())
	assert.ErrorIs(t, err, ErrContractNotFound)
}
```

**Step 2: Write test helpers (setupMySQL)**

Create `services/contracts/cmd/server/testhelpers_test.go`:

```go
package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/copycatsh/hire-flow/services/contracts/migrations"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	_ "github.com/go-sql-driver/mysql"
)

func setupMySQL(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	mysqlContainer, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("test_contracts"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mysqlContainer.Terminate(context.Background()) })

	connStr, err := mysqlContainer.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)

	db, err := sql.Open("mysql", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("mysql"))
	require.NoError(t, goose.Up(db, "."))

	return db
}
```

**Step 3: Run tests to verify they fail**

Run: `cd services/contracts && go test ./cmd/server/ -run TestMySQLContractStore -v -count=1`
Expected: FAIL — `MySQLContractStore` not defined

**Step 4: Implement contract store**

```go
// services/contracts/cmd/server/contract_store.go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrContractNotFound = errors.New("contract not found")
var ErrStatusConflict = errors.New("contract status conflict")

type MySQLContractStore struct{}

func (s *MySQLContractStore) Create(ctx context.Context, db DBTX, c Contract) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO contracts (id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ClientID, c.FreelancerID, c.Title, c.Description, c.Amount, c.Currency, c.Status, c.ClientWalletID, c.FreelancerWalletID,
	)
	if err != nil {
		return fmt.Errorf("contract create: %w", err)
	}
	return nil
}

func (s *MySQLContractStore) GetByID(ctx context.Context, db DBTX, id string) (Contract, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id, hold_id, created_at, updated_at
		 FROM contracts WHERE id = ?`, id,
	)

	var c Contract
	err := row.Scan(&c.ID, &c.ClientID, &c.FreelancerID, &c.Title, &c.Description, &c.Amount, &c.Currency, &c.Status, &c.ClientWalletID, &c.FreelancerWalletID, &c.HoldID, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Contract{}, ErrContractNotFound
	}
	if err != nil {
		return Contract{}, fmt.Errorf("contract get by id: %w", err)
	}
	return c, nil
}

func (s *MySQLContractStore) UpdateStatus(ctx context.Context, db DBTX, id string, from string, to string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE contracts SET status = ? WHERE id = ? AND status = ?`,
		to, id, from,
	)
	if err != nil {
		return fmt.Errorf("contract update status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("contract update status rows: %w", err)
	}
	if rows == 0 {
		return ErrStatusConflict
	}
	return nil
}

func (s *MySQLContractStore) SetHoldID(ctx context.Context, db DBTX, id string, holdID string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE contracts SET hold_id = ? WHERE id = ?`,
		holdID, id,
	)
	if err != nil {
		return fmt.Errorf("contract set hold_id: %w", err)
	}
	return nil
}
```

**Step 5: Run tests to verify they pass**

Run: `cd services/contracts && go test ./cmd/server/ -run TestMySQLContractStore -v -count=1`
Expected: PASS (all 4 tests)

**Step 6: Implement milestone store**

```go
// services/contracts/cmd/server/milestone_store.go
package main

import (
	"context"
	"fmt"
)

type MySQLMilestoneStore struct{}

func (s *MySQLMilestoneStore) CreateBatch(ctx context.Context, db DBTX, milestones []Milestone) error {
	for _, m := range milestones {
		_, err := db.ExecContext(ctx,
			`INSERT INTO milestones (id, contract_id, title, description, amount, position, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.ContractID, m.Title, m.Description, m.Amount, m.Position, m.Status,
		)
		if err != nil {
			return fmt.Errorf("milestone create: %w", err)
		}
	}
	return nil
}

func (s *MySQLMilestoneStore) ListByContract(ctx context.Context, db DBTX, contractID string) ([]Milestone, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, contract_id, title, description, amount, position, status, created_at, updated_at
		 FROM milestones WHERE contract_id = ? ORDER BY position`, contractID,
	)
	if err != nil {
		return nil, fmt.Errorf("milestone list: %w", err)
	}
	defer rows.Close()

	var milestones []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.ContractID, &m.Title, &m.Description, &m.Amount, &m.Position, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("milestone scan: %w", err)
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}
```

**Step 7: Commit**

```bash
git add services/contracts/cmd/server/contract_store.go services/contracts/cmd/server/contract_store_test.go services/contracts/cmd/server/milestone_store.go services/contracts/cmd/server/testhelpers_test.go
git commit -m "feat(m4): MySQL store implementations — contracts + milestones"
```

---

## Task 4: MySQL outbox store + publisher

**Why:** Contracts needs its own outbox for MySQL (pkg/outbox is Postgres-only). Both store and publisher are contracts-local.

**Files:**
- Create: `services/contracts/cmd/server/outbox.go`
- Create: `services/contracts/cmd/server/outbox_publisher.go`

**Step 1: Write outbox store test**

Create `services/contracts/cmd/server/outbox_test.go`:

```go
package main

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLOutboxStore_InsertAndFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLOutboxStore{}

	payload, _ := json.Marshal(map[string]string{"test": "data"})
	entry := OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   uuid.New().String(),
		EventType:     EventContractCreated,
		Payload:       payload,
	}

	require.NoError(t, store.Insert(ctx, db, entry))

	entries, err := store.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, entry.ID, entries[0].ID)
	assert.Equal(t, EventContractCreated, entries[0].EventType)
}

func TestMySQLOutboxStore_MarkPublished(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	store := &MySQLOutboxStore{}

	payload, _ := json.Marshal(map[string]string{"test": "data"})
	entry := OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   uuid.New().String(),
		EventType:     EventContractCreated,
		Payload:       payload,
	}
	require.NoError(t, store.Insert(ctx, db, entry))

	require.NoError(t, store.MarkPublished(ctx, db, []string{entry.ID}))

	entries, err := store.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd services/contracts && go test ./cmd/server/ -run TestMySQLOutbox -v -count=1`
Expected: FAIL — `MySQLOutboxStore` not defined

**Step 3: Implement outbox store**

```go
// services/contracts/cmd/server/outbox.go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type OutboxEntry struct {
	ID            string     `json:"id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	Payload       []byte     `json:"payload"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitzero"`
}

// OutboxStore persists and queries outbox entries in MySQL.
type OutboxStore interface {
	Insert(ctx context.Context, db DBTX, entry OutboxEntry) error
	FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error)
	MarkPublished(ctx context.Context, db DBTX, ids []string) error
}

// EventPublisher sends events to a message broker.
type EventPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

type MySQLOutboxStore struct{}

func (s *MySQLOutboxStore) Insert(ctx context.Context, db DBTX, entry OutboxEntry) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.AggregateType, entry.AggregateID, entry.EventType, entry.Payload,
	)
	if err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}
	return nil
}

func (s *MySQLOutboxStore) FetchUnpublished(ctx context.Context, db DBTX, limit int) ([]OutboxEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at, published_at
		 FROM outbox
		 WHERE published_at IS NULL
		 ORDER BY created_at
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox fetch unpublished: %w", err)
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType, &e.Payload, &e.CreatedAt, &e.PublishedAt); err != nil {
			return nil, fmt.Errorf("outbox scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *MySQLOutboxStore) MarkPublished(ctx context.Context, db DBTX, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	_, err := db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE outbox SET published_at = CURRENT_TIMESTAMP(6) WHERE id IN (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return fmt.Errorf("outbox mark published: %w", err)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd services/contracts && go test ./cmd/server/ -run TestMySQLOutbox -v -count=1`
Expected: PASS

**Step 5: Implement outbox publisher**

```go
// services/contracts/cmd/server/outbox_publisher.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// OutboxPublisher polls the MySQL outbox table and publishes entries to NATS.
//
//	┌──────────┐   TX   ┌──────────┐
//	│ Handler   │──────▶│ outbox   │
//	└──────────┘        └────┬─────┘
//	                         │ poll
//	                    ┌────▼─────┐       ┌──────────┐
//	                    │ Publisher │──────▶│  NATS    │
//	                    └──────────┘       └──────────┘

type OutboxPublisher struct {
	store    OutboxStore
	db       *sql.DB
	pub      EventPublisher
	interval time.Duration
	batch    int
}

func NewOutboxPublisher(store OutboxStore, db *sql.DB, pub EventPublisher, interval time.Duration) *OutboxPublisher {
	return &OutboxPublisher{
		store:    store,
		db:       db,
		pub:      pub,
		interval: interval,
		batch:    100,
	}
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.PublishBatch(ctx); err != nil {
				slog.Error("outbox publish batch", "error", err)
			}
		}
	}
}

func (p *OutboxPublisher) PublishBatch(ctx context.Context) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("outbox begin tx: %w", err)
	}
	defer tx.Rollback()

	entries, err := p.store.FetchUnpublished(ctx, tx, p.batch)
	if err != nil {
		return fmt.Errorf("outbox fetch unpublished: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	var published []string
	for _, e := range entries {
		if err := p.pub.Publish(ctx, e.EventType, e.Payload); err != nil {
			slog.Error("outbox: publish to nats", "error", err, "entry_id", e.ID)
			continue
		}
		published = append(published, e.ID)
	}

	if len(published) > 0 {
		if err := p.store.MarkPublished(ctx, tx, published); err != nil {
			return fmt.Errorf("outbox mark published: %w", err)
		}
	}

	return tx.Commit()
}
```

**Step 6: Commit**

```bash
git add services/contracts/cmd/server/outbox.go services/contracts/cmd/server/outbox_publisher.go services/contracts/cmd/server/outbox_test.go
git commit -m "feat(m4): MySQL outbox store + publisher for contracts"
```

---

## Task 5: NATS client + Payments HTTP client

**Why:** The saga needs two I/O channels: NATS (events) and HTTP (Payments commands). Build these before the saga orchestrator.

**Files:**
- Create: `services/contracts/cmd/server/nats.go`
- Create: `services/contracts/cmd/server/payments_client.go`

**Step 1: Implement NATS client**

```go
// services/contracts/cmd/server/nats.go
package main

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSClient wraps NATS JetStream for publishing and consuming.
type NATSClient struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

func NewNATSClient(url string) (*NATSClient, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}

	return &NATSClient{conn: conn, js: js}, nil
}

func (c *NATSClient) EnsureStream(ctx context.Context) error {
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "CONTRACTS",
		Subjects:  []string{"contracts.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("nats ensure CONTRACTS stream: %w", err)
	}
	return nil
}

func (c *NATSClient) EnsurePaymentsConsumer(ctx context.Context) (jetstream.Consumer, error) {
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, "PAYMENTS", jetstream.ConsumerConfig{
		Durable:       "contracts-saga",
		FilterSubjects: []string{
			"payments.payment.held",
			"payments.payment.released",
			"payments.payment.transferred",
			"payments.payment.failed",
		},
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("nats ensure payments consumer: %w", err)
	}
	return consumer, nil
}

func (c *NATSClient) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := c.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}
	return nil
}

func (c *NATSClient) Close() {
	c.conn.Close()
}
```

**Step 2: Implement Payments HTTP client**

```go
// services/contracts/cmd/server/payments_client.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type PaymentsClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaymentsClient(baseURL string) *PaymentsClient {
	return &PaymentsClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

type HoldRequest struct {
	WalletID   string `json:"wallet_id"`
	Amount     int64  `json:"amount"`
	ContractID string `json:"contract_id"`
}

type HoldResponse struct {
	ID         string `json:"id"`
	WalletID   string `json:"wallet_id"`
	Amount     int64  `json:"amount"`
	Status     string `json:"status"`
	ContractID string `json:"contract_id"`
}

type ReleaseRequest struct {
	HoldID string `json:"hold_id"`
}

type TransferRequest struct {
	HoldID            string `json:"hold_id"`
	RecipientWalletID string `json:"recipient_wallet_id"`
}

func (c *PaymentsClient) HoldFunds(ctx context.Context, req HoldRequest) (HoldResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/hold", bytes.NewReader(body))
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return HoldResponse{}, fmt.Errorf("payments hold failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var holdResp HoldResponse
	if err := json.Unmarshal(respBody, &holdResp); err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold unmarshal: %w", err)
	}
	return holdResp, nil
}

func (c *PaymentsClient) ReleaseFunds(ctx context.Context, holdID string) error {
	body, err := json.Marshal(ReleaseRequest{HoldID: holdID})
	if err != nil {
		return fmt.Errorf("payments release marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/release", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payments release request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("payments release call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payments release failed: status=%d body=%s", resp.StatusCode, respBody)
	}
	return nil
}

func (c *PaymentsClient) TransferFunds(ctx context.Context, holdID string, recipientWalletID string) error {
	body, err := json.Marshal(TransferRequest{HoldID: holdID, RecipientWalletID: recipientWalletID})
	if err != nil {
		return fmt.Errorf("payments transfer marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/transfer", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payments transfer request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("payments transfer call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payments transfer failed: status=%d body=%s", resp.StatusCode, respBody)
	}
	return nil
}
```

**Step 3: Commit**

```bash
git add services/contracts/cmd/server/nats.go services/contracts/cmd/server/payments_client.go
git commit -m "feat(m4): NATS client + Payments HTTP client"
```

---

## Task 6: Saga orchestrator

**Why:** The core M4 feature — coordinates contract lifecycle across services.

**Files:**
- Create: `services/contracts/cmd/server/saga.go`

**Step 1: Write saga tests**

Create `services/contracts/cmd/server/saga_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockPaymentsServer(t *testing.T, holdStatus int, holdResp HoldResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(holdStatus)
			json.NewEncoder(w).Encode(holdResp)
		case r.URL.Path == "/api/v1/payments/release" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/payments/transfer" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSaga_CreateContract_HoldSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusCreated, HoldResponse{
		ID:       holdID,
		WalletID: uuid.New().String(),
		Amount:   50000,
		Status:   "active",
	})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Build API",
		Amount:             50000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}

	result, err := saga.CreateContract(ctx, c, nil)
	require.NoError(t, err)
	assert.Equal(t, StatusHoldPending, result.Status)
	assert.Equal(t, &holdID, result.HoldID)
}

func TestSaga_HandlePaymentHeld(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusHoldPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.HandlePaymentHeld(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusAwaitingAccept, got.Status)
}

func TestSaga_HandlePaymentFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusHoldPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.HandlePaymentFailed(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, got.Status)
}

func TestSaga_AcceptContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusAwaitingAccept,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.AcceptContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractAccepted, entries[0].EventType)
}

func TestSaga_CancelContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusAwaitingAccept,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CancelContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusDeclined, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractDeclined, entries[0].EventType)
}

func TestSaga_CompleteContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	freelancerWalletID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusActive,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: freelancerWalletID,
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CompleteContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)

	// Verify outbox event
	entries, err := outboxStore.FetchUnpublished(ctx, db, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, EventContractCompleted, entries[0].EventType)
}

func TestSaga_AcceptContract_WrongStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, outboxStore, nil)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusPending,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	err := saga.AcceptContract(ctx, contractID)
	assert.ErrorIs(t, err, ErrStatusConflict)
}

func TestSaga_CompleteContract_RetryFromCompleting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusOK, HoldResponse{})
	defer ps.Close()

	paymentsClient := NewPaymentsClient(ps.URL)
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, paymentsClient)

	contractID := uuid.New().String()
	c := Contract{
		ID:                 contractID,
		ClientID:           uuid.New().String(),
		FreelancerID:       uuid.New().String(),
		Title:              "Test",
		Amount:             10000,
		Currency:           "USD",
		Status:             StatusCompleting,
		ClientWalletID:     uuid.New().String(),
		FreelancerWalletID: uuid.New().String(),
		HoldID:             &holdID,
	}
	require.NoError(t, contractStore.Create(ctx, db, c))
	require.NoError(t, contractStore.SetHoldID(ctx, db, contractID, holdID))

	err := saga.CompleteContract(ctx, contractID)
	require.NoError(t, err)

	got, err := contractStore.GetByID(ctx, db, contractID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd services/contracts && go test ./cmd/server/ -run TestSaga -v -count=1`
Expected: FAIL — `NewSagaOrchestrator` not defined

**Step 3: Implement saga orchestrator**

```go
// services/contracts/cmd/server/saga.go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// SagaOrchestrator coordinates the contract lifecycle across services.
//
// Forward flow:
//   CreateContract → PENDING → call Payments hold → HOLD_PENDING
//   payment.held event → AWAITING_ACCEPT
//   AcceptContract → ACTIVE (+ outbox: contract.accepted)
//   CompleteContract → COMPLETING → call Payments transfer → COMPLETED (+ outbox: contract.completed)
//
// Compensation flow:
//   payment.failed event → CANCELLED (+ outbox: contract.cancelled)
//   CancelContract → DECLINING → call Payments release → DECLINED (+ outbox: contract.declined)
//
// Retry: CompleteContract and CancelContract handle both fresh calls and retries
//   from COMPLETING/DECLINING states (idempotent).

type SagaOrchestrator struct {
	db       *sql.DB
	contracts ContractStore
	outbox    OutboxStore
	payments  *PaymentsClient
}

func NewSagaOrchestrator(db *sql.DB, contracts ContractStore, outbox OutboxStore, payments *PaymentsClient) *SagaOrchestrator {
	return &SagaOrchestrator{
		db:        db,
		contracts: contracts,
		outbox:    outbox,
		payments:  payments,
	}
}

// CreateContract creates a contract and immediately calls Payments to hold funds.
// Returns the contract in HOLD_PENDING status on success.
func (s *SagaOrchestrator) CreateContract(ctx context.Context, c Contract, milestones []Milestone) (Contract, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: begin tx: %w", err)
	}
	defer tx.Rollback()

	c.ID = uuid.New().String()
	c.Status = StatusPending
	c.Currency = "USD"

	if err := s.contracts.Create(ctx, tx, c); err != nil {
		return Contract{}, fmt.Errorf("saga create: insert contract: %w", err)
	}

	if len(milestones) > 0 {
		ms := &MySQLMilestoneStore{}
		for i := range milestones {
			milestones[i].ID = uuid.New().String()
			milestones[i].ContractID = c.ID
			milestones[i].Status = "PENDING"
		}
		if err := ms.CreateBatch(ctx, tx, milestones); err != nil {
			return Contract{}, fmt.Errorf("saga create: insert milestones: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Contract{}, fmt.Errorf("saga create: commit: %w", err)
	}

	// Call Payments to hold funds (outside transaction — HTTP call)
	holdResp, err := s.payments.HoldFunds(ctx, HoldRequest{
		WalletID:   c.ClientWalletID,
		Amount:     c.Amount,
		ContractID: c.ID,
	})
	if err != nil {
		// Hold failed — mark contract cancelled
		slog.Error("saga create: hold failed", "error", err, "contract_id", c.ID)
		cancelTx, txErr := s.db.BeginTx(ctx, nil)
		if txErr == nil {
			_ = s.contracts.UpdateStatus(ctx, cancelTx, c.ID, StatusPending, StatusCancelled)
			payload, _ := json.Marshal(c)
			_ = s.outbox.Insert(ctx, cancelTx, OutboxEntry{
				ID:            uuid.New().String(),
				AggregateType: "contract",
				AggregateID:   c.ID,
				EventType:     EventContractCancelled,
				Payload:       payload,
			})
			_ = cancelTx.Commit()
		}
		c.Status = StatusCancelled
		return c, fmt.Errorf("saga create: hold funds: %w", err)
	}

	// Hold succeeded — update contract to HOLD_PENDING with hold_id
	updateTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: begin update tx: %w", err)
	}
	defer updateTx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, updateTx, c.ID, StatusPending, StatusHoldPending); err != nil {
		return Contract{}, fmt.Errorf("saga create: update to hold_pending: %w", err)
	}
	if err := s.contracts.SetHoldID(ctx, updateTx, c.ID, holdResp.ID); err != nil {
		return Contract{}, fmt.Errorf("saga create: set hold_id: %w", err)
	}

	payload, err := json.Marshal(c)
	if err != nil {
		return Contract{}, fmt.Errorf("saga create: marshal payload: %w", err)
	}
	if err := s.outbox.Insert(ctx, updateTx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   c.ID,
		EventType:     EventContractCreated,
		Payload:       payload,
	}); err != nil {
		return Contract{}, fmt.Errorf("saga create: outbox insert: %w", err)
	}

	if err := updateTx.Commit(); err != nil {
		return Contract{}, fmt.Errorf("saga create: commit update: %w", err)
	}

	c.Status = StatusHoldPending
	c.HoldID = &holdResp.ID
	return c, nil
}

// HandlePaymentHeld advances contract from HOLD_PENDING → AWAITING_ACCEPT.
// Called by the NATS consumer when payment.held event is received.
func (s *SagaOrchestrator) HandlePaymentHeld(ctx context.Context, contractID string) error {
	return s.contracts.UpdateStatus(ctx, s.db, contractID, StatusHoldPending, StatusAwaitingAccept)
}

// HandlePaymentFailed marks contract as CANCELLED when hold fails.
// Called by the NATS consumer when payment.failed event is received.
func (s *SagaOrchestrator) HandlePaymentFailed(ctx context.Context, contractID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga payment failed: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusHoldPending, StatusCancelled); err != nil {
		return fmt.Errorf("saga payment failed: update status: %w", err)
	}

	c, err := s.contracts.GetByID(ctx, tx, contractID)
	if err != nil {
		return fmt.Errorf("saga payment failed: get contract: %w", err)
	}

	payload, _ := json.Marshal(c)
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractCancelled,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga payment failed: outbox insert: %w", err)
	}

	return tx.Commit()
}

// AcceptContract transitions AWAITING_ACCEPT → ACTIVE.
func (s *SagaOrchestrator) AcceptContract(ctx context.Context, contractID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga accept: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusAwaitingAccept, StatusActive); err != nil {
		return fmt.Errorf("saga accept: update status: %w", err)
	}

	c, err := s.contracts.GetByID(ctx, tx, contractID)
	if err != nil {
		return fmt.Errorf("saga accept: get contract: %w", err)
	}

	payload, _ := json.Marshal(c)
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractAccepted,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga accept: outbox insert: %w", err)
	}

	return tx.Commit()
}

// CompleteContract transitions ACTIVE → COMPLETING → calls transfer → COMPLETED.
// Also handles retry from COMPLETING (idempotent).
func (s *SagaOrchestrator) CompleteContract(ctx context.Context, contractID string) error {
	c, err := s.contracts.GetByID(ctx, s.db, contractID)
	if err != nil {
		return fmt.Errorf("saga complete: get contract: %w", err)
	}

	if c.Status == StatusActive {
		if err := s.contracts.UpdateStatus(ctx, s.db, contractID, StatusActive, StatusCompleting); err != nil {
			return fmt.Errorf("saga complete: update to completing: %w", err)
		}
		c.Status = StatusCompleting
	}

	if c.Status != StatusCompleting {
		return ErrStatusConflict
	}

	if c.HoldID == nil {
		return fmt.Errorf("saga complete: no hold_id on contract %s", contractID)
	}

	if err := s.payments.TransferFunds(ctx, *c.HoldID, c.FreelancerWalletID); err != nil {
		return fmt.Errorf("saga complete: transfer: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga complete: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusCompleting, StatusCompleted); err != nil {
		return fmt.Errorf("saga complete: update to completed: %w", err)
	}

	c.Status = StatusCompleted
	payload, _ := json.Marshal(c)
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractCompleted,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga complete: outbox insert: %w", err)
	}

	return tx.Commit()
}

// CancelContract transitions AWAITING_ACCEPT → DECLINING → calls release → DECLINED.
// Also handles retry from DECLINING (idempotent).
func (s *SagaOrchestrator) CancelContract(ctx context.Context, contractID string) error {
	c, err := s.contracts.GetByID(ctx, s.db, contractID)
	if err != nil {
		return fmt.Errorf("saga cancel: get contract: %w", err)
	}

	if c.Status == StatusAwaitingAccept {
		if err := s.contracts.UpdateStatus(ctx, s.db, contractID, StatusAwaitingAccept, StatusDeclining); err != nil {
			return fmt.Errorf("saga cancel: update to declining: %w", err)
		}
		c.Status = StatusDeclining
	}

	if c.Status != StatusDeclining {
		return ErrStatusConflict
	}

	if c.HoldID == nil {
		return fmt.Errorf("saga cancel: no hold_id on contract %s", contractID)
	}

	if err := s.payments.ReleaseFunds(ctx, *c.HoldID); err != nil {
		return fmt.Errorf("saga cancel: release: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga cancel: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.contracts.UpdateStatus(ctx, tx, contractID, StatusDeclining, StatusDeclined); err != nil {
		return fmt.Errorf("saga cancel: update to declined: %w", err)
	}

	c.Status = StatusDeclined
	payload, _ := json.Marshal(c)
	if err := s.outbox.Insert(ctx, tx, OutboxEntry{
		ID:            uuid.New().String(),
		AggregateType: "contract",
		AggregateID:   contractID,
		EventType:     EventContractDeclined,
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("saga cancel: outbox insert: %w", err)
	}

	return tx.Commit()
}
```

**Step 4: Run tests to verify they pass**

Run: `cd services/contracts && go test ./cmd/server/ -run TestSaga -v -count=1`
Expected: PASS (all 8 tests)

**Step 5: Commit**

```bash
git add services/contracts/cmd/server/saga.go services/contracts/cmd/server/saga_test.go
git commit -m "feat(m4): saga orchestrator — contract lifecycle with compensation"
```

---

## Task 7: Chi HTTP handlers

**Why:** REST API layer — delegates to saga orchestrator for state transitions.

**Files:**
- Create: `services/contracts/cmd/server/handler.go`

**Step 1: Write handler tests**

Create `services/contracts/cmd/server/handler_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRouter(h *ContractHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestHandler_CreateContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	holdID := uuid.New().String()
	ps := mockPaymentsServer(t, http.StatusCreated, HoldResponse{
		ID:       holdID,
		WalletID: uuid.New().String(),
		Amount:   50000,
		Status:   "active",
	})
	defer ps.Close()

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLOutboxStore{}, NewPaymentsClient(ps.URL))
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Build API",
		"description": "REST API development",
		"amount": 50000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `",
		"milestones": [
			{"title": "Phase 1", "description": "Design", "amount": 20000, "position": 0},
			{"title": "Phase 2", "description": "Implement", "amount": 30000, "position": 1}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, StatusHoldPending, resp.Status)
	assert.NotEmpty(t, resp.ID)
}

func TestHandler_CreateContract_ValidationError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	body := `{"title": "Missing fields"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_AcceptContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}

	saga := NewSagaOrchestrator(db, contractStore, outboxStore, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	contractID := uuid.New().String()
	c := Contract{
		ID: contractID, ClientID: uuid.New().String(), FreelancerID: uuid.New().String(),
		Title: "Test", Amount: 10000, Currency: "USD", Status: StatusAwaitingAccept,
		ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+contractID+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, StatusActive, resp.Status)
}

func TestHandler_AcceptContract_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	saga := NewSagaOrchestrator(db, &MySQLContractStore{}, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: &MySQLContractStore{}, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+uuid.New().String()+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_AcceptContract_WrongStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	contractStore := &MySQLContractStore{}

	saga := NewSagaOrchestrator(db, contractStore, &MySQLOutboxStore{}, nil)
	h := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := newTestRouter(h)

	contractID := uuid.New().String()
	c := Contract{
		ID: contractID, ClientID: uuid.New().String(), FreelancerID: uuid.New().String(),
		Title: "Test", Amount: 10000, Currency: "USD", Status: StatusPending,
		ClientWalletID: uuid.New().String(), FreelancerWalletID: uuid.New().String(),
	}
	require.NoError(t, contractStore.Create(ctx, db, c))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+contractID+"/accept", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd services/contracts && go test ./cmd/server/ -run TestHandler -v -count=1`
Expected: FAIL — `ContractHandler` not defined

**Step 3: Implement handler**

```go
// services/contracts/cmd/server/handler.go
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ContractHandler struct {
	saga       *SagaOrchestrator
	contracts  ContractStore
	milestones MilestoneStore
	db         *sql.DB
}

func (h *ContractHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/v1/contracts", func(r chi.Router) {
		r.Post("/", h.CreateContract)
		r.Put("/{id}/accept", h.AcceptContract)
		r.Put("/{id}/complete", h.CompleteContract)
		r.Put("/{id}/cancel", h.CancelContract)
		r.Get("/{id}", h.GetContract)
	})
}

func (h *ContractHandler) CreateContract(w http.ResponseWriter, r *http.Request) {
	var req CreateContractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ClientID == "" || req.FreelancerID == "" || req.Title == "" || req.Amount <= 0 || req.ClientWalletID == "" || req.FreelancerWalletID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id, freelancer_id, title, amount, client_wallet_id, freelancer_wallet_id are required"})
		return
	}

	c := Contract{
		ClientID:           req.ClientID,
		FreelancerID:       req.FreelancerID,
		Title:              req.Title,
		Description:        req.Description,
		Amount:             req.Amount,
		ClientWalletID:     req.ClientWalletID,
		FreelancerWalletID: req.FreelancerWalletID,
	}

	var milestones []Milestone
	for _, ms := range req.Milestones {
		milestones = append(milestones, Milestone{
			Title:       ms.Title,
			Description: ms.Description,
			Amount:      ms.Amount,
			Position:    ms.Position,
		})
	}

	result, err := h.saga.CreateContract(r.Context(), c, milestones)
	if err != nil {
		slog.Error("create contract", "error", err)
		if result.Status == StatusCancelled {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "payment hold failed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (h *ContractHandler) AcceptContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("accept contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.AcceptContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be accepted in current status"})
			return
		}
		slog.Error("accept contract", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	c, _ := h.contracts.GetByID(r.Context(), h.db, id)
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) CompleteContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("complete contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.CompleteContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be completed in current status"})
			return
		}
		slog.Error("complete contract", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment service unavailable, retry later"})
		return
	}

	c, _ := h.contracts.GetByID(r.Context(), h.db, id)
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) CancelContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, err := h.contracts.GetByID(r.Context(), h.db, id); err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("cancel contract: get", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := h.saga.CancelContract(r.Context(), id); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "contract cannot be cancelled in current status"})
			return
		}
		slog.Error("cancel contract", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "payment service unavailable, retry later"})
		return
	}

	c, _ := h.contracts.GetByID(r.Context(), h.db, id)
	writeJSON(w, http.StatusOK, c)
}

func (h *ContractHandler) GetContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	c, err := h.contracts.GetByID(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, ErrContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "contract not found"})
			return
		}
		slog.Error("get contract", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, c)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd services/contracts && go test ./cmd/server/ -run TestHandler -v -count=1`
Expected: PASS (all 5 tests)

**Step 5: Commit**

```bash
git add services/contracts/cmd/server/handler.go services/contracts/cmd/server/handler_test.go
git commit -m "feat(m4): Chi HTTP handlers — create, accept, complete, cancel, get"
```

---

## Task 8: NATS consumer + main.go wiring

**Why:** Wire everything together: MySQL, NATS, Chi router, outbox publisher, saga consumer, graceful shutdown.

**Files:**
- Modify: `services/contracts/cmd/server/main.go`

**Step 1: Rewrite main.go**

```go
// services/contracts/cmd/server/main.go
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

	"github.com/copycatsh/hire-flow/services/contracts/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pressly/goose/v3"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// MySQL
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Error("open mysql", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Run migrations
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("mysql"); err != nil {
		slog.Error("goose set dialect", "error", err)
		os.Exit(1)
	}
	if err := goose.Up(db, "."); err != nil {
		slog.Error("goose up", "error", err)
		os.Exit(1)
	}

	// NATS
	nc, err := NewNATSClient(natsURL)
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.EnsureStream(ctx); err != nil {
		slog.Error("nats ensure contracts stream", "error", err)
		os.Exit(1)
	}

	paymentsConsumer, err := nc.EnsurePaymentsConsumer(ctx)
	if err != nil {
		slog.Error("nats ensure payments consumer", "error", err)
		os.Exit(1)
	}

	// Stores
	contractStore := &MySQLContractStore{}
	milestoneStore := &MySQLMilestoneStore{}
	outboxStore := &MySQLOutboxStore{}

	// Saga + handler
	paymentsClient := NewPaymentsClient(paymentsURL)
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, paymentsClient)

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
	srv := &http.Server{Addr: port, Handler: r}
	go func() {
		slog.Info("starting contracts", "port", port)
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
			slog.Error("nats fetch", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			subject := msg.Subject()
			slog.Info("received payment event", "subject", subject)

			var payload struct {
				ContractID string `json:"contract_id"`
			}
			if err := json.Unmarshal(msg.Data(), &payload); err != nil {
				slog.Error("nats unmarshal", "error", err, "subject", subject)
				msg.Nak()
				continue
			}

			if payload.ContractID == "" {
				slog.Warn("nats event missing contract_id", "subject", subject)
				msg.Ack()
				continue
			}

			var handleErr error
			switch subject {
			case "payments.payment.held":
				handleErr = saga.HandlePaymentHeld(ctx, payload.ContractID)
			case "payments.payment.failed":
				handleErr = saga.HandlePaymentFailed(ctx, payload.ContractID)
			default:
				slog.Info("ignoring payment event", "subject", subject)
			}

			if handleErr != nil {
				slog.Error("saga handle event", "error", handleErr, "subject", subject, "contract_id", payload.ContractID)
				msg.Nak()
				continue
			}

			msg.Ack()
		}
	}
}
```

**Step 2: Update go.mod dependencies**

Run:
```bash
cd services/contracts && go get github.com/go-sql-driver/mysql@latest github.com/pressly/goose/v3@latest github.com/nats-io/nats.go@latest github.com/google/uuid@latest github.com/stretchr/testify@latest github.com/testcontainers/testcontainers-go/modules/mysql@latest github.com/testcontainers/testcontainers-go/modules/nats@latest && go mod tidy
```

**Step 3: Verify build**

Run: `cd services/contracts && go build ./cmd/server/`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add services/contracts/cmd/server/main.go services/contracts/go.mod services/contracts/go.sum
git commit -m "feat(m4): wire main.go — MySQL, NATS, Chi, outbox publisher, saga consumer"
```

---

## Task 9: Update compose.yaml + Dockerfile

**Why:** Add environment variables and NATS dependency to contracts service. Update Dockerfile for multi-module build (needs migrations embed).

**Files:**
- Modify: `compose.yaml`
- Modify: `services/contracts/Dockerfile`

**Step 1: Update compose.yaml**

Update the contracts service block to match the payments pattern:

```yaml
  contracts:
    build:
      context: .
      dockerfile: services/contracts/Dockerfile
    container_name: hire-flow-contracts
    ports:
      - "8003:8003"
    environment:
      DATABASE_URL: hire_flow:hire_flow_dev@tcp(mysql:3306)/contracts_db?parseTime=true
      NATS_URL: nats://nats:4222
      PAYMENTS_URL: http://payments:8004
    networks:
      - hire-flow
    depends_on:
      mysql:
        condition: service_healthy
      nats:
        condition: service_healthy
      payments:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8003/health > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5
```

**Step 2: Update Dockerfile**

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY services/contracts/go.mod services/contracts/go.sum ./services/contracts/
RUN cd services/contracts && go mod download
COPY services/contracts/ ./services/contracts/
RUN cd services/contracts && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8003
CMD ["/server"]
```

**Step 3: Verify Docker build**

Run: `cd /Users/anton/WorkProjects/pet_projects/hire-flow && docker compose build contracts`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add compose.yaml services/contracts/Dockerfile
git commit -m "feat(m4): update compose.yaml + Dockerfile for contracts service"
```

---

## Task 10: Integration tests — full saga flows

**Why:** End-to-end verification of the saga happy path and compensation flows. Uses testcontainers for MySQL + NATS and mock Payments HTTP.

**Files:**
- Create: `services/contracts/cmd/server/integration_test.go`

**Step 1: Write integration tests**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

func setupNATS(t *testing.T, ctx context.Context) *NATSClient {
	t.Helper()

	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := NewNATSClient(natsURL)
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	require.NoError(t, nc.EnsureStream(ctx))
	return nc
}

func TestIntegration_FullHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)
	nc := setupNATS(t, ctx)

	holdID := uuid.New().String()
	var transferCalled atomic.Bool
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(HoldResponse{
				ID: holdID, WalletID: uuid.New().String(), Amount: 50000, Status: "active",
			})
		case r.URL.Path == "/api/v1/payments/transfer":
			transferCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. Create contract
	clientWalletID := uuid.New().String()
	freelancerWalletID := uuid.New().String()
	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Full Flow Test",
		"description": "E2E test",
		"amount": 50000,
		"client_wallet_id": "` + clientWalletID + `",
		"freelancer_wallet_id": "` + freelancerWalletID + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, StatusHoldPending, created.Status)

	// 2. Simulate payment.held event → AWAITING_ACCEPT
	require.NoError(t, saga.HandlePaymentHeld(ctx, created.ID))
	got, _ := contractStore.GetByID(ctx, db, created.ID)
	assert.Equal(t, StatusAwaitingAccept, got.Status)

	// 3. Accept → ACTIVE
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/accept", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 4. Complete → COMPLETED (calls transfer)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/complete", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var completed Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &completed))
	assert.Equal(t, StatusCompleted, completed.Status)
	assert.True(t, transferCalled.Load(), "transfer should have been called")

	// 5. Verify outbox has events
	entries, err := outboxStore.FetchUnpublished(ctx, db, 100)
	require.NoError(t, err)
	eventTypes := make([]string, len(entries))
	for i, e := range entries {
		eventTypes[i] = e.EventType
	}
	assert.Contains(t, eventTypes, EventContractCreated)
	assert.Contains(t, eventTypes, EventContractAccepted)
	assert.Contains(t, eventTypes, EventContractCompleted)

	// 6. Verify outbox publisher works with NATS
	publisher := NewOutboxPublisher(outboxStore, db, nc, time.Second)
	require.NoError(t, publisher.PublishBatch(ctx))

	// Verify events published to NATS
	rawConn, err := nats.Connect(nc.conn.ConnectedUrl())
	require.NoError(t, err)
	defer rawConn.Close()

	js, err := jetstream.New(rawConn)
	require.NoError(t, err)

	consumer, err := js.CreateConsumer(ctx, "CONTRACTS", jetstream.ConsumerConfig{
		FilterSubject: "contracts.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	msgs, err := consumer.Fetch(3, jetstream.FetchMaxWait(5*time.Second))
	require.NoError(t, err)

	var received int
	for range msgs.Messages() {
		received++
	}
	assert.Equal(t, 3, received, "should have received 3 events in NATS")
}

func TestIntegration_CompensationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	holdID := uuid.New().String()
	var releaseCalled atomic.Bool
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/payments/hold":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(HoldResponse{
				ID: holdID, WalletID: uuid.New().String(), Amount: 30000, Status: "active",
			})
		case r.URL.Path == "/api/v1/payments/release":
			releaseCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// 1. Create contract
	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Compensation Test",
		"amount": 30000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// 2. Simulate payment.held → AWAITING_ACCEPT
	require.NoError(t, saga.HandlePaymentHeld(ctx, created.ID))

	// 3. Cancel → DECLINING → release → DECLINED
	req = httptest.NewRequest(http.MethodPut, "/api/v1/contracts/"+created.ID+"/cancel", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var declined Contract
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &declined))
	assert.Equal(t, StatusDeclined, declined.Status)
	assert.True(t, releaseCalled.Load(), "release should have been called")

	// Verify outbox
	entries, err := outboxStore.FetchUnpublished(ctx, db, 100)
	require.NoError(t, err)
	eventTypes := make([]string, len(entries))
	for i, e := range entries {
		eventTypes[i] = e.EventType
	}
	assert.Contains(t, eventTypes, EventContractCreated)
	assert.Contains(t, eventTypes, EventContractDeclined)
}

func TestIntegration_PaymentFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	db := setupMySQL(t, ctx)

	// Payments returns 422 (insufficient funds)
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": "insufficient funds"})
	}))
	defer ps.Close()

	contractStore := &MySQLContractStore{}
	outboxStore := &MySQLOutboxStore{}
	saga := NewSagaOrchestrator(db, contractStore, outboxStore, NewPaymentsClient(ps.URL))
	handler := &ContractHandler{saga: saga, contracts: contractStore, milestones: &MySQLMilestoneStore{}, db: db}
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{
		"client_id": "` + uuid.New().String() + `",
		"freelancer_id": "` + uuid.New().String() + `",
		"title": "Failed Payment Test",
		"amount": 100000,
		"client_wallet_id": "` + uuid.New().String() + `",
		"freelancer_wallet_id": "` + uuid.New().String() + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Verify contract was created as cancelled
	var contracts []Contract
	rows, err := db.QueryContext(ctx, `SELECT id, status FROM contracts ORDER BY created_at DESC LIMIT 1`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var c Contract
		require.NoError(t, rows.Scan(&c.ID, &c.Status))
		contracts = append(contracts, c)
	}
	require.Len(t, contracts, 1)
	assert.Equal(t, StatusCancelled, contracts[0].Status)
}
```

**Step 2: Run all tests**

Run: `cd services/contracts && go test ./cmd/server/ -v -count=1`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add services/contracts/cmd/server/integration_test.go
git commit -m "feat(m4): integration tests — happy path, compensation, payment failure"
```

---

## Task 11: Update TODOS.md + smoke test

**Why:** Add the 3 deferred TODOs from eng review. Verify everything works with Docker Compose.

**Files:**
- Modify: `TODOS.md`

**Step 1: Add new TODOs**

Add to TODOS.md under Pending:

```markdown
### Add milestone-level completion to contracts
- **What:** Extend `PUT /contracts/{id}/milestones/{mid}/complete` for individual milestone completion with proportional payment transfer
- **Why:** M4 ships contract-level completion (all-or-nothing). Real contracts need incremental milestone payments.
- **Context:** Milestones table exists with `amount` and `status` columns. Schema is forward-compatible. Saga orchestrator needs per-milestone transfer logic.
- **Depends on:** M4 complete

### Add background retry for stuck saga states
- **What:** Background goroutine retries contracts in COMPLETING/DECLINING for > 5 minutes by re-calling Payments HTTP
- **Why:** M4 uses idempotent manual retry. Without background retry, contracts stay stuck if no one retries. Safety net for saga failures.
- **Context:** Similar to hold expiry TODO — both are saga recovery safety nets. Could be combined into single "saga recovery" job.
- **Depends on:** M4 complete

### Add 48h acceptance timeout for contracts
- **What:** Background job auto-cancels contracts in AWAITING_ACCEPT for > 48 hours, triggering compensation (release hold)
- **Why:** Architecture spec says "WaitForAcceptance (timeout: 48h)". Without this, ignored contracts hold funds indefinitely.
- **Context:** Payments hold `expires_at` column is a safety net. This timeout is business-logic enforcement.
- **Depends on:** M4 complete
```

**Step 2: Smoke test with Docker Compose**

Run:
```bash
cd /Users/anton/WorkProjects/pet_projects/hire-flow && make up && sleep 10 && make health
```
Expected: All services healthy including contracts on port 8003.

**Step 3: Commit**

```bash
git add TODOS.md
git commit -m "feat(m4): add deferred TODOs — milestone completion, saga retry, acceptance timeout"
```

---

## Summary

| Task | What | Files |
|---|---|---|
| 1 | MySQL migrations | 4 new |
| 2 | Domain types + interfaces | 4 new |
| 3 | MySQL store implementations | 4 new (2 impl + 2 test) |
| 4 | MySQL outbox store + publisher | 3 new (2 impl + 1 test) |
| 5 | NATS client + Payments HTTP client | 2 new |
| 6 | Saga orchestrator | 2 new (1 impl + 1 test) |
| 7 | Chi HTTP handlers | 2 new (1 impl + 1 test) |
| 8 | Main.go wiring + NATS consumer | 1 modified + go.mod |
| 9 | compose.yaml + Dockerfile | 2 modified |
| 10 | Integration tests | 1 new |
| 11 | TODOS.md + smoke test | 1 modified |

**Total:** ~15 new files, 3 modified files, 11 commits.
