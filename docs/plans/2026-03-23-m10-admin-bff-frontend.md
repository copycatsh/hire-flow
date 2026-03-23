# M10: Admin BFF + Admin Frontend — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Admin users can login, view all jobs, all contracts, all wallets, and platform stats via a dedicated admin frontend.

**Architecture:** Expand the existing admin BFF skeleton (:8012) with auth + downstream service clients following the exact pattern from client/freelancer BFFs. Add a new Vite frontend app at :5175. Wrap all downstream list responses in `{items, total}` format and update existing frontends atomically. Add `RequireRole("admin")` middleware to `pkg/bff`.

**Tech Stack:** Go 1.25 stdlib (admin BFF), React 19 + TanStack Router + React Query + Tailwind v4 (frontend), pkg/bff shared auth/middleware/client

---

## Phase 1: Downstream Service Changes (list response format + new endpoints)

### Task 1: Wrap jobs-api list response in `{items, total}`

**Files:**
- Modify: `services/jobs-api/cmd/server/job_store.go:41-80` (add Count method)
- Modify: `services/jobs-api/cmd/server/job.go:45-49` (add ListResponse type)
- Modify: `services/jobs-api/cmd/server/job_handler.go:81-112` (wrap response)
- Modify: `services/jobs-api/cmd/server/job_handler_test.go` (update test assertions)

**Step 1: Add ListResponse type and Count to store interface**

In `services/jobs-api/cmd/server/job.go`, add after `ListJobsParams` (line 49):

```go
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
```

In `services/jobs-api/cmd/server/job.go`, update `JobStore` interface (line 51-56) to add:

```go
Count(ctx context.Context, db DBTX, params ListJobsParams) (int, error)
```

**Step 2: Implement Count in store**

In `services/jobs-api/cmd/server/job_store.go`, add after the `List` method (after line 80):

```go
func (s *PostgresJobStore) Count(ctx context.Context, db DBTX, params ListJobsParams) (int, error) {
	var (
		query strings.Builder
		args  []any
		argN  int
	)

	query.WriteString(`SELECT COUNT(*) FROM jobs`)

	if params.Status != nil {
		argN++
		query.WriteString(` WHERE status = $` + strconv.Itoa(argN))
		args = append(args, *params.Status)
	}

	var count int
	err := db.QueryRow(ctx, query.String(), args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("job count: %w", err)
	}
	return count, nil
}
```

**Step 3: Update List handler to return wrapped response**

In `services/jobs-api/cmd/server/job_handler.go`, replace the List method body (lines 81-112):

```go
func (h *JobHandler) List(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	params := ListJobsParams{
		Limit:  limit,
		Offset: offset,
	}
	if s := c.QueryParam("status"); s != "" {
		if !validJobStatuses[s] {
			return NewAppError(http.StatusBadRequest, "invalid status filter")
		}
		params.Status = &s
	}

	ctx := c.Request().Context()

	jobs, err := h.jobs.List(ctx, h.pool, params)
	if err != nil {
		return err
	}
	if jobs == nil {
		jobs = []Job{}
	}

	total, err := h.jobs.Count(ctx, h.pool, params)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, ListResponse[Job]{Items: jobs, Total: total})
}
```

**Step 4: Update existing tests**

In `services/jobs-api/cmd/server/job_handler_test.go`, update `TestListJobs_Success` to expect `{items: [...], total: N}` format. Assertions should read from `.Items` field.

**Step 5: Run tests**

Run: `cd services/jobs-api && go test ./cmd/server/ -v -run TestList -short`
Expected: PASS

**Step 6: Commit**

```bash
git add services/jobs-api/cmd/server/job.go services/jobs-api/cmd/server/job_store.go services/jobs-api/cmd/server/job_handler.go services/jobs-api/cmd/server/job_handler_test.go
git commit -m "feat(jobs-api): wrap list response in {items, total}"
```

---

### Task 2: Allow unfiltered contract list + wrap response in `{items, total}`

**Files:**
- Modify: `services/contracts/cmd/server/contract.go:84-97` (add ListResponse, Count to interface)
- Modify: `services/contracts/cmd/server/contract_store.go:62-107` (add Count method)
- Modify: `services/contracts/cmd/server/handler.go:191-231` (allow empty filter, wrap response)
- Modify: `services/contracts/cmd/server/handler_test.go` (update tests)

**Step 1: Add ListResponse type and Count to interface**

In `services/contracts/cmd/server/contract.go`, add after `ListFilter` (line 89):

```go
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
```

Update `ContractStore` interface (line 91-97) to add:

```go
Count(ctx context.Context, db DBTX, filter ListFilter) (int, error)
```

**Step 2: Implement Count in store**

In `services/contracts/cmd/server/contract_store.go`, add after the `List` method (after line 107):

```go
func (s *MySQLContractStore) Count(ctx context.Context, db DBTX, filter ListFilter) (int, error) {
	query := `SELECT COUNT(*) FROM contracts WHERE 1=1`
	args := []any{}

	if filter.ClientID != "" {
		query += " AND client_id = ?"
		args = append(args, filter.ClientID)
	}
	if filter.FreelancerID != "" {
		query += " AND freelancer_id = ?"
		args = append(args, filter.FreelancerID)
	}

	var count int
	err := db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("contract count: %w", err)
	}
	return count, nil
}
```

**Step 3: Update ListContracts handler — allow unfiltered + wrap response**

In `services/contracts/cmd/server/handler.go`, replace `ListContracts` (lines 191-231):

```go
func (h *ContractHandler) ListContracts(w http.ResponseWriter, r *http.Request) {
	filter := ListFilter{
		ClientID:     r.URL.Query().Get("client_id"),
		FreelancerID: r.URL.Query().Get("freelancer_id"),
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit parameter"})
			return
		}
		filter.Limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid offset parameter"})
			return
		}
		filter.Offset = n
	}

	contracts, err := h.contracts.List(r.Context(), h.db, filter)
	if err != nil {
		slog.ErrorContext(r.Context(), "list contracts", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if contracts == nil {
		contracts = []Contract{}
	}

	total, err := h.contracts.Count(r.Context(), h.db, filter)
	if err != nil {
		slog.ErrorContext(r.Context(), "count contracts", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, ListResponse[Contract]{Items: contracts, Total: total})
}
```

Note: the `client_id or freelancer_id required` check on line 197-200 is **removed** — no filter means return all.

**Step 4: Update existing tests**

- `TestHandler_ListContracts_MissingFilter` — should now return 200 with all contracts instead of 400
- `TestHandler_ListContracts_ByClientID` — assert response is `{items: [...], total: N}`
- `TestHandler_ListContracts_ByFreelancerID` — same
- `TestHandler_ListContracts_EmptyResult` — assert `{items: [], total: 0}`

**Step 5: Run tests**

Run: `cd services/contracts && go test ./cmd/server/ -v -run TestHandler_List -short`
Expected: PASS

**Step 6: Commit**

```bash
git add services/contracts/cmd/server/
git commit -m "feat(contracts): allow unfiltered list, wrap response in {items, total}"
```

---

### Task 3: Add list-all-wallets endpoint + wrap wallet response

**Files:**
- Modify: `services/payments/cmd/server/wallet.go:20-25` (add ListAll + Count to interface)
- Modify: `services/payments/cmd/server/wallet_store.go` (add ListAll + Count methods)
- Modify: `services/payments/cmd/server/handler.go:24-30` (register new route)
- Modify: `services/payments/cmd/server/handler.go` (add ListWallets handler)
- Modify: `services/payments/cmd/server/handler_test.go` (add tests)

**Step 1: Add ListResponse type, ListAll and Count to store**

In `services/payments/cmd/server/wallet.go`, add after `Wallet` struct (after line 18):

```go
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
```

Update `WalletStore` interface (lines 20-25) to add:

```go
ListAll(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error)
Count(ctx context.Context, db DBTX) (int, error)
```

**Step 2: Implement ListAll and Count in store**

In `services/payments/cmd/server/wallet_store.go`, add after `Seed` method (after line 79):

```go
func (s *PostgresWalletStore) ListAll(ctx context.Context, db DBTX, limit, offset int) ([]Wallet, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := db.Query(ctx,
		`SELECT w.id, w.user_id, w.balance, w.currency,
		        w.balance - COALESCE(h.held, 0) AS available_balance,
		        w.created_at, w.updated_at
		 FROM wallets w
		 LEFT JOIN (
		     SELECT wallet_id, SUM(amount) AS held
		     FROM holds
		     WHERE status = 'active'
		     GROUP BY wallet_id
		 ) h ON h.wallet_id = w.id
		 ORDER BY w.created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("wallet list all: %w", err)
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		if err := rows.Scan(&w.ID, &w.UserID, &w.Balance, &w.Currency, &w.AvailableBalance, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("wallet list scan: %w", err)
		}
		wallets = append(wallets, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wallet list rows: %w", err)
	}
	return wallets, nil
}

func (s *PostgresWalletStore) Count(ctx context.Context, db DBTX) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT COUNT(*) FROM wallets`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("wallet count: %w", err)
	}
	return count, nil
}
```

**Step 3: Register route and add handler**

In `services/payments/cmd/server/handler.go`, add the new route in `RegisterRoutes` (line 29):

```go
g.GET("/wallets", h.ListWallets)
```

Add the handler method after `GetWallet` (after line 413):

```go
func (h *PaymentHandler) ListWallets(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = n
	}

	offset := 0
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		offset = n
	}

	wallets, err := h.wallets.ListAll(c.Request.Context(), h.pool, limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "list wallets", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if wallets == nil {
		wallets = []Wallet{}
	}

	total, err := h.wallets.Count(c.Request.Context(), h.pool)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "count wallets", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, ListResponse[Wallet]{Items: wallets, Total: total})
}
```

Note: add `"strconv"` to imports in handler.go if not already present.

**Step 4: Write tests for ListWallets**

In `services/payments/cmd/server/handler_test.go`, add:

```go
func TestListWallets_Empty(t *testing.T) {
	// Setup with test pool, seed no wallets
	// GET /api/v1/payments/wallets
	// Assert 200 with {items: [], total: 0}
}

func TestListWallets_WithData(t *testing.T) {
	// Seed 2 wallets
	// GET /api/v1/payments/wallets
	// Assert 200 with {items: [w1, w2], total: 2}
}
```

Follow existing test patterns in this file (uses gin test context + httptest.NewRecorder).

**Step 5: Run tests**

Run: `cd services/payments && go test ./cmd/server/ -v -run TestListWallets -short`
Expected: PASS

**Step 6: Commit**

```bash
git add services/payments/cmd/server/
git commit -m "feat(payments): add list-all-wallets endpoint with {items, total} response"
```

---

## Phase 2: RequireRole Middleware

### Task 4: Add RequireRole middleware to pkg/bff

**Files:**
- Modify: `pkg/bff/context.go` (add RequireRole function)
- Create: `pkg/bff/role_test.go` (tests)

**Step 1: Write the failing tests**

Create `pkg/bff/role_test.go`:

```go
package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/copycatsh/hire-flow/pkg/bff"
	"github.com/stretchr/testify/assert"
)

func TestRequireRole_Allows(t *testing.T) {
	handler := bff.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), bff.CtxKeyRole, "admin")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_RejectsWrongRole(t *testing.T) {
	handler := bff.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), bff.CtxKeyRole, "client")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	assert.Contains(t, body["error"], "forbidden")
}

func TestRequireRole_RejectsMissingRole(t *testing.T) {
	handler := bff.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd pkg/bff && go test -v -run TestRequireRole`
Expected: FAIL — `RequireRole` not defined

**Step 3: Implement RequireRole**

In `pkg/bff/context.go`, add after `RoleFrom` (after line 24):

```go
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if RoleFrom(r.Context()) != role {
				WriteError(w, http.StatusForbidden, "forbidden: requires "+role+" role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd pkg/bff && go test -v -run TestRequireRole`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add pkg/bff/context.go pkg/bff/role_test.go
git commit -m "feat(bff): add RequireRole middleware"
```

---

## Phase 3: Admin BFF

### Task 5: Expand admin BFF with auth, clients, and handlers

**Files:**
- Modify: `services/bff/admin/cmd/server/main.go` (full rewrite from skeleton)
- Create: `services/bff/admin/cmd/server/job_handler.go`
- Create: `services/bff/admin/cmd/server/contract_handler.go`
- Create: `services/bff/admin/cmd/server/wallet_handler.go`
- Create: `services/bff/admin/cmd/server/dashboard_handler.go`
- Modify: `services/bff/admin/go.mod` (add pkg/bff dependency)
- Modify: `services/bff/admin/Dockerfile` (add pkg/bff COPY)

**Step 1: Update go.mod to include pkg/bff**

In `services/bff/admin/go.mod`, add `pkg/bff` as a require + replace:

```
require (
	github.com/copycatsh/hire-flow/pkg/bff v0.0.0-00010101000000-000000000000
	github.com/copycatsh/hire-flow/pkg/telemetry v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0
)

replace (
	github.com/copycatsh/hire-flow/pkg/bff => ../../../pkg/bff
	github.com/copycatsh/hire-flow/pkg/telemetry => ../../../pkg/telemetry
)
```

Then run: `cd services/bff/admin && go mod tidy`

**Step 2: Update Dockerfile to include pkg/bff**

Replace `services/bff/admin/Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/bff/go.mod pkg/bff/go.sum ./pkg/bff/
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/bff/admin/go.mod services/bff/admin/go.sum ./services/bff/admin/
RUN cd services/bff/admin && go mod download
COPY pkg/bff/ ./pkg/bff/
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/bff/admin/ ./services/bff/admin/
RUN cd services/bff/admin && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8012
CMD ["/server"]
```

**Step 3: Rewrite main.go**

Replace `services/bff/admin/cmd/server/main.go` entirely — follow the exact pattern from `services/bff/client/cmd/server/main.go`:

```go
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
	port := cmp.Or(os.Getenv("PORT"), "8012")
	if port[0] != ':' {
		port = ":" + port
	}
	jwtSecret := cmp.Or(os.Getenv("JWT_SECRET"), "dev-secret-change-in-production")
	jobsURL := cmp.Or(os.Getenv("JOBS_URL"), "http://jobs-api:8001")
	contractsURL := cmp.Or(os.Getenv("CONTRACTS_URL"), "http://contracts:8003")
	paymentsURL := cmp.Or(os.Getenv("PAYMENTS_URL"), "http://payments:8004")
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")
	cookieSecure := os.Getenv("COOKIE_SECURE") != "false"

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, "bff-admin", otelEndpoint)
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
	jobsClient := &bff.ServiceClient{BaseURL: jobsURL, HTTP: httpClient, Name: "jobs-api"}
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

	jobHandler := &JobHandler{jobs: jobsClient}
	jobHandler.RegisterRoutes(apiMux)

	contractHandler := &ContractHandler{contracts: contractsClient}
	contractHandler.RegisterRoutes(apiMux)

	walletHandler := &WalletHandler{payments: paymentsClient}
	walletHandler.RegisterRoutes(apiMux)

	dashboardHandler := &DashboardHandler{
		jobs:      jobsClient,
		contracts: contractsClient,
		payments:  paymentsClient,
	}
	dashboardHandler.RegisterRoutes(apiMux)

	protected := auth.JWTMiddleware(bff.RequireRole("admin")(rateLimiter.Middleware(apiMux)))
	mux.Handle("/api/", protected)

	handler := otelhttp.NewHandler(bff.RequestLogger(mux), "bff-admin")

	srv := &http.Server{
		Addr:         port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.InfoContext(ctx, "starting bff-admin", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down bff-admin")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "shutdown error", "error", err)
	}
}
```

Key difference from client BFF: `RequireRole("admin")` middleware in the chain, no matching client.

**Step 4: Create job_handler.go**

Create `services/bff/admin/cmd/server/job_handler.go`:

```go
package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type JobHandler struct {
	jobs *bff.ServiceClient
}

func (h *JobHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/jobs", h.List)
	mux.HandleFunc("GET /api/v1/jobs/{id}", h.GetByID)
}

func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/jobs"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.jobs.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *JobHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.jobs.Forward(r.Context(), w, http.MethodGet, "/api/v1/jobs/"+id, nil)
}
```

Note: admin can view all jobs (no user filter), no create/update (read-only).

**Step 5: Create contract_handler.go**

Create `services/bff/admin/cmd/server/contract_handler.go`:

```go
package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type ContractHandler struct {
	contracts *bff.ServiceClient
}

func (h *ContractHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/contracts", h.List)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetByID)
}

func (h *ContractHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/contracts"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.contracts.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *ContractHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodGet, "/api/v1/contracts/"+id, nil)
}
```

Note: no user filter — admin sees all contracts.

**Step 6: Create wallet_handler.go**

Create `services/bff/admin/cmd/server/wallet_handler.go`:

```go
package main

import (
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type WalletHandler struct {
	payments *bff.ServiceClient
}

func (h *WalletHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallets", h.List)
}

func (h *WalletHandler) List(w http.ResponseWriter, r *http.Request) {
	path := "/api/v1/payments/wallets"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, path, nil)
}
```

**Step 7: Create dashboard_handler.go**

Create `services/bff/admin/cmd/server/dashboard_handler.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type DashboardHandler struct {
	jobs      *bff.ServiceClient
	contracts *bff.ServiceClient
	payments  *bff.ServiceClient
}

func (h *DashboardHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dashboard/stats", h.Stats)
}

type serviceStats struct {
	Total int    `json:"total"`
	Error string `json:"error,omitzero"`
}

type dashboardResponse struct {
	Jobs      serviceStats `json:"jobs"`
	Contracts serviceStats `json:"contracts"`
	Wallets   serviceStats `json:"wallets"`
}

func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var resp dashboardResponse
	var mu sync.Mutex
	var wg sync.WaitGroup

	type listResp struct {
		Total int `json:"total"`
	}

	fetch := func(client *bff.ServiceClient, path string, target *serviceStats) {
		defer wg.Done()
		var result listResp
		err := client.Do(ctx, http.MethodGet, path, nil, &result)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			target.Error = client.Name + " unavailable"
			return
		}
		target.Total = result.Total
	}

	wg.Add(3)
	go fetch(h.jobs, "/api/v1/jobs?limit=1", &resp.Jobs)
	go fetch(h.contracts, "/api/v1/contracts?limit=1", &resp.Contracts)
	go fetch(h.payments, "/api/v1/payments/wallets?limit=1", &resp.Wallets)
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

Note: calls 3 services in parallel with `limit=1` to get `total` from each `{items, total}` response. Partial failure returns error string per service.

**Step 8: Run go mod tidy and verify build**

Run: `cd services/bff/admin && go mod tidy && go build ./cmd/server/`
Expected: Build succeeds

**Step 9: Commit**

```bash
git add services/bff/admin/
git commit -m "feat(bff-admin): add auth, handlers for jobs, contracts, wallets, dashboard"
```

---

### Task 6: Admin BFF handler tests

**Files:**
- Create: `services/bff/admin/cmd/server/handler_test.go`

**Step 1: Write tests**

Create `services/bff/admin/cmd/server/handler_test.go` following the pattern from `services/bff/client/cmd/server/handler_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/copycatsh/hire-flow/pkg/bff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMux(t *testing.T, jobsSrv, contractsSrv, paymentsSrv *httptest.Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

	if jobsSrv != nil {
		jh := &JobHandler{jobs: &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"}}
		jh.RegisterRoutes(mux)
	}
	if contractsSrv != nil {
		ch := &ContractHandler{contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"}}
		ch.RegisterRoutes(mux)
	}
	if paymentsSrv != nil {
		wh := &WalletHandler{payments: &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"}}
		wh.RegisterRoutes(mux)
	}

	return mux
}

func adminRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), bff.CtxKeyUserID, "44444444-4444-4444-4444-444444444444")
	ctx = context.WithValue(ctx, bff.CtxKeyRole, "admin")
	return req.WithContext(ctx)
}

func TestJobHandler_List_ForwardsAll(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
	}))
	defer srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/jobs", gotPath)
}

func TestContractHandler_List_NoFilter(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, srv, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/contracts", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, gotQuery)
}

func TestWalletHandler_List(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
	}))
	defer srv.Close()

	mux := setupTestMux(t, nil, nil, srv)
	req := adminRequest(t, http.MethodGet, "/api/v1/wallets", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/api/v1/payments/wallets", gotPath)
}

func TestDashboard_Stats_AllServicesUp(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 5})
	}))
	defer jobsSrv.Close()

	contractsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 3})
	}))
	defer contractsSrv.Close()

	paymentsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 2})
	}))
	defer paymentsSrv.Close()

	dh := &DashboardHandler{
		jobs:      &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"},
		contracts: &bff.ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"},
		payments:  &bff.ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"},
	}

	mux := http.NewServeMux()
	dh.RegisterRoutes(mux)

	req := adminRequest(t, http.MethodGet, "/api/v1/dashboard/stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp dashboardResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, 5, resp.Jobs.Total)
	assert.Equal(t, 3, resp.Contracts.Total)
	assert.Equal(t, 2, resp.Wallets.Total)
	assert.Empty(t, resp.Jobs.Error)
}

func TestDashboard_Stats_PartialFailure(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 5})
	}))
	defer jobsSrv.Close()

	downSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	downSrv.Close()

	dh := &DashboardHandler{
		jobs:      &bff.ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"},
		contracts: &bff.ServiceClient{BaseURL: downSrv.URL, HTTP: downSrv.Client(), Name: "contracts"},
		payments:  &bff.ServiceClient{BaseURL: downSrv.URL, HTTP: downSrv.Client(), Name: "payments"},
	}

	mux := http.NewServeMux()
	dh.RegisterRoutes(mux)

	req := adminRequest(t, http.MethodGet, "/api/v1/dashboard/stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp dashboardResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, 5, resp.Jobs.Total)
	assert.Empty(t, resp.Jobs.Error)
	assert.NotEmpty(t, resp.Contracts.Error)
	assert.NotEmpty(t, resp.Wallets.Error)
}

func TestServiceDown_Returns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	mux := setupTestMux(t, srv, nil, nil)
	req := adminRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
```

**Step 2: Run tests**

Run: `cd services/bff/admin && go test ./cmd/server/ -v`
Expected: PASS (7 tests)

**Step 3: Commit**

```bash
git add services/bff/admin/cmd/server/handler_test.go
git commit -m "test(bff-admin): add handler tests for all endpoints + dashboard partial failure"
```

---

## Phase 4: Infrastructure Updates

### Task 7: Update compose.yaml and Traefik config

**Files:**
- Modify: `compose.yaml:369-387` (update bff-admin)
- Modify: `infra/traefik/dynamic.yml:39-41` (add :5175 to CORS origins)

**Step 1: Update compose.yaml bff-admin section**

Replace the bff-admin service block (lines 369-387) with:

```yaml
  bff-admin:
    build:
      context: .
      dockerfile: services/bff/admin/Dockerfile
    container_name: hire-flow-bff-admin
    environment:
      JWT_SECRET: "dev-secret-change-in-production"
      JOBS_URL: "http://jobs-api:8001"
      CONTRACTS_URL: "http://contracts:8003"
      PAYMENTS_URL: "http://payments:8004"
      OTEL_ENDPOINT: "otel-collector:4317"
      COOKIE_SECURE: "false"
    ports:
      - "8012:8012"
    networks:
      - hire-flow
    depends_on:
      jobs-api:
        condition: service_healthy
      contracts:
        condition: service_healthy
      payments:
        condition: service_healthy
      otel-collector:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8012/health > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5
```

**Step 2: Add localhost:5175 to Traefik CORS**

In `infra/traefik/dynamic.yml`, add `"http://localhost:5175"` to the `accessControlAllowOriginList` (after line 41):

```yaml
        accessControlAllowOriginList:
          - "http://localhost:5173"
          - "http://localhost:5174"
          - "http://localhost:5175"
```

**Step 3: Commit**

```bash
git add compose.yaml infra/traefik/dynamic.yml
git commit -m "infra: update admin BFF compose config, add :5175 to CORS"
```

---

## Phase 5: Admin Frontend

### Task 8: Scaffold admin Vite app

**Files:**
- Create: `services/frontend/apps/admin/package.json`
- Create: `services/frontend/apps/admin/tsconfig.json`
- Create: `services/frontend/apps/admin/tsconfig.app.json`
- Create: `services/frontend/apps/admin/vite.config.ts`
- Create: `services/frontend/apps/admin/vitest.config.ts`
- Create: `services/frontend/apps/admin/index.html`
- Create: `services/frontend/apps/admin/src/main.tsx`
- Create: `services/frontend/apps/admin/src/lib/api-client.ts`
- Create: `services/frontend/apps/admin/src/test/setup.ts`

**Step 1: Create package.json**

Create `services/frontend/apps/admin/package.json`:

```json
{
  "name": "@hire-flow/admin",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "@hire-flow/ui": "workspace:*",
    "@tanstack/react-query": "^5.64.2",
    "@tanstack/react-router": "^1.95.0",
    "@tanstack/router-devtools": "^1.95.0",
    "@tanstack/router-plugin": "^1.95.0",
    "lucide-react": "^0.577.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.2.2",
    "@testing-library/jest-dom": "^6.6.0",
    "@testing-library/react": "^16.1.0",
    "@testing-library/user-event": "^14.5.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.0",
    "jsdom": "^25.0.0",
    "msw": "^2.7.0",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^6.0.0",
    "vitest": "^3.0.0"
  }
}
```

**Step 2: Create tsconfig files**

`services/frontend/apps/admin/tsconfig.json`:

```json
{
  "files": [],
  "references": [{ "path": "./tsconfig.app.json" }]
}
```

`services/frontend/apps/admin/tsconfig.app.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src"]
}
```

**Step 3: Create vite.config.ts**

`services/frontend/apps/admin/vite.config.ts`:

```typescript
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import path from "node:path";

export default defineConfig({
  plugins: [tailwindcss(), TanStackRouterVite({ quoteStyle: "double" }), react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5175,
    proxy: {
      "/admin": {
        target: "http://localhost:80",
        changeOrigin: true,
      },
    },
  },
});
```

**Step 4: Create vitest.config.ts**

`services/frontend/apps/admin/vitest.config.ts`:

```typescript
import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
    conditions: ["@tanstack/custom-condition"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
```

**Step 5: Create index.html**

`services/frontend/apps/admin/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>hire-flow admin</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&family=Instrument+Sans:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
</head>
<body>
  <div id="root"></div>
  <script type="module" src="/src/main.tsx"></script>
</body>
</html>
```

**Step 6: Create main.tsx**

`services/frontend/apps/admin/src/main.tsx`:

```typescript
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { AuthState } from "@hire-flow/ui";
import { routeTree } from "./routeTree.gen";
import "@hire-flow/ui/globals.css";

const auth = new AuthState("/admin");

const router = createRouter({
  routeTree,
  context: { auth },
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const root = document.getElementById("root");
if (!root) throw new Error("Root element not found");

createRoot(root).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
```

Note: `AuthState("/admin")` — different base URL than client ("/client") or freelancer ("/freelancer").

**Step 7: Create api-client.ts**

`services/frontend/apps/admin/src/lib/api-client.ts`:

```typescript
import { createApiClient } from "@hire-flow/ui";
export const apiClient = createApiClient("/admin");
```

**Step 8: Create test setup**

`services/frontend/apps/admin/src/test/setup.ts`:

```typescript
import "@testing-library/jest-dom/vitest";
```

**Step 9: Install dependencies**

Run: `cd services/frontend && pnpm install`
Expected: Dependencies installed including new admin workspace package

**Step 10: Commit**

```bash
git add services/frontend/apps/admin/
git commit -m "feat(frontend-admin): scaffold Vite app with TanStack Router + auth"
```

---

### Task 9: Admin frontend routes and auth layout

**Files:**
- Create: `services/frontend/apps/admin/src/routes/__root.tsx`
- Create: `services/frontend/apps/admin/src/routes/login.tsx`
- Create: `services/frontend/apps/admin/src/routes/_auth.tsx`
- Create: `services/frontend/apps/admin/src/features/auth/login-page.tsx`
- Create: `services/frontend/apps/admin/src/features/auth/use-login.ts`
- Create: `services/frontend/apps/admin/src/features/auth/auth-context.tsx`

**Step 1: Create __root.tsx**

`services/frontend/apps/admin/src/routes/__root.tsx`:

```typescript
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, type AuthState } from "@hire-flow/ui";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

interface RouterContext {
  auth: AuthState;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context }) => {
    await context.auth.restore();
  },
  component: function RootLayout() {
    const { auth } = Route.useRouteContext();
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider auth={auth}>
          <Outlet />
        </AuthProvider>
      </QueryClientProvider>
    );
  },
});
```

**Step 2: Create login.tsx route**

`services/frontend/apps/admin/src/routes/login.tsx`:

```typescript
import { createFileRoute } from "@tanstack/react-router";
import { LoginPage } from "@/features/auth/login-page";

export const Route = createFileRoute("/login")({
  component: LoginPage,
});
```

**Step 3: Create _auth.tsx layout**

`services/frontend/apps/admin/src/routes/_auth.tsx`:

```typescript
import { createFileRoute, Outlet, redirect, Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { TopNav } from "@hire-flow/ui";
import { LayoutDashboard, Briefcase, FileText, Wallet } from "lucide-react";

const navItems = [
  { to: "/", label: "Dashboard", icon: <LayoutDashboard size={16} /> },
  { to: "/jobs", label: "Jobs", icon: <Briefcase size={16} /> },
  { to: "/contracts", label: "Contracts", icon: <FileText size={16} /> },
  { to: "/wallets", label: "Wallets", icon: <Wallet size={16} /> },
];

export const Route = createFileRoute("/_auth")({
  beforeLoad: ({ context }) => {
    if (!context.auth.user) {
      throw redirect({ to: "/login" });
    }
  },
  component: AuthLayout,
});

function AuthLayout() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <div className="min-h-screen bg-background-subtle">
      <TopNav
        appName="admin"
        navItems={navItems}
        currentPath={pathname}
        avatar="AD"
        onLogout={logout}
        renderLink={({ to, className, children }) => (
          <Link to={to} className={className}>{children}</Link>
        )}
      />
      <main className="mx-auto max-w-7xl px-8 py-12">
        <Outlet />
      </main>
    </div>
  );
}
```

Note: appName is `"admin"` — displays as "hireadmin" in logo. Dashboard is the first nav item at `/`.

**Step 4: Create auth re-exports**

`services/frontend/apps/admin/src/features/auth/auth-context.tsx`:

```typescript
export { AuthProvider, useAuth } from "@hire-flow/ui";
```

`services/frontend/apps/admin/src/features/auth/use-login.ts`:

```typescript
import { useLogin as useLoginBase } from "@hire-flow/ui";
import { apiClient } from "@/lib/api-client";

export function useLogin() {
  return useLoginBase(apiClient);
}
```

**Step 5: Create login page**

`services/frontend/apps/admin/src/features/auth/login-page.tsx`:

```typescript
import { useState, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useLogin } from "./use-login";

export function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useLogin();
  const navigate = useNavigate();

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate(
      { email, password },
      {
        onSuccess: () => navigate({ to: "/" }),
      },
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background-subtle">
      <div className="w-full max-w-sm rounded-lg border border-border bg-background p-8 shadow-sm">
        <h1 className="font-display text-2xl font-bold tracking-tight">
          hire<span className="text-primary">admin</span>
        </h1>
        <p className="mt-1 text-sm text-foreground-secondary">Admin Portal</p>

        {login.isError && (
          <div className="mt-4 rounded-md bg-error-bg px-4 py-2 text-sm text-error">
            {login.error.message}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-6 space-y-4">
          <div>
            <label className="text-sm font-medium text-foreground" htmlFor="email">Email</label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="admin@example.com"
              className="mt-1 w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <div>
            <label className="text-sm font-medium text-foreground" htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter your password"
              className="mt-1 w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <button
            type="submit"
            disabled={login.isPending}
            className="w-full rounded-sm bg-primary px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
          >
            {login.isPending ? "Signing in..." : "Sign In"}
          </button>
        </form>
      </div>
    </div>
  );
}
```

**Step 6: Commit**

```bash
git add services/frontend/apps/admin/src/
git commit -m "feat(frontend-admin): add routes, auth layout, login page"
```

---

### Task 10: Admin feature pages — Dashboard, Jobs, Contracts, Wallets

**Files:**
- Create: `services/frontend/apps/admin/src/features/dashboard/types.ts`
- Create: `services/frontend/apps/admin/src/features/dashboard/queries.ts`
- Create: `services/frontend/apps/admin/src/features/dashboard/dashboard-page.tsx`
- Create: `services/frontend/apps/admin/src/features/jobs/types.ts`
- Create: `services/frontend/apps/admin/src/features/jobs/queries.ts`
- Create: `services/frontend/apps/admin/src/features/jobs/job-list.tsx`
- Create: `services/frontend/apps/admin/src/features/contracts/types.ts`
- Create: `services/frontend/apps/admin/src/features/contracts/queries.ts`
- Create: `services/frontend/apps/admin/src/features/contracts/contract-list.tsx`
- Create: `services/frontend/apps/admin/src/features/wallets/types.ts`
- Create: `services/frontend/apps/admin/src/features/wallets/queries.ts`
- Create: `services/frontend/apps/admin/src/features/wallets/wallet-list.tsx`
- Create: `services/frontend/apps/admin/src/routes/_auth/index.tsx`
- Create: `services/frontend/apps/admin/src/routes/_auth/jobs.index.tsx`
- Create: `services/frontend/apps/admin/src/routes/_auth/contracts.index.tsx`
- Create: `services/frontend/apps/admin/src/routes/_auth/wallets.index.tsx`

**Step 1: Dashboard feature**

`services/frontend/apps/admin/src/features/dashboard/types.ts`:

```typescript
export interface ServiceStats {
  total: number;
  error?: string;
}

export interface DashboardStats {
  jobs: ServiceStats;
  contracts: ServiceStats;
  wallets: ServiceStats;
}
```

`services/frontend/apps/admin/src/features/dashboard/queries.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { DashboardStats } from "./types";

export function useDashboardStats() {
  return useQuery({
    queryKey: ["dashboard-stats"],
    queryFn: () => apiClient.get<DashboardStats>("/api/v1/dashboard/stats"),
  });
}
```

`services/frontend/apps/admin/src/features/dashboard/dashboard-page.tsx`:

```typescript
import { useDashboardStats } from "./queries";
import type { ServiceStats } from "./types";

function StatCard({ label, stats }: { label: string; stats: ServiceStats }) {
  return (
    <div className="rounded-md border border-border bg-background p-6 shadow-sm">
      <p className="text-sm font-medium text-foreground-secondary">{label}</p>
      {stats.error ? (
        <p className="mt-2 text-sm text-error">{stats.error}</p>
      ) : (
        <p className="mt-2 font-mono text-3xl font-semibold text-foreground">{stats.total}</p>
      )}
    </div>
  );
}

export function DashboardPage() {
  const { data, isLoading, isError, error } = useDashboardStats();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading dashboard...
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">
        {error.message}
      </div>
    );
  }

  if (!data) return null;

  return (
    <div>
      <h1 className="mb-8 font-display text-2xl font-semibold tracking-tight">
        Platform Overview
      </h1>
      <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <StatCard label="Total Jobs" stats={data.jobs} />
        <StatCard label="Total Contracts" stats={data.contracts} />
        <StatCard label="Total Wallets" stats={data.wallets} />
      </div>
    </div>
  );
}
```

**Step 2: Jobs feature**

`services/frontend/apps/admin/src/features/jobs/types.ts`:

```typescript
export interface Job {
  id: string;
  title: string;
  description: string;
  budget_min: number;
  budget_max: number;
  status: "draft" | "open" | "in_progress" | "closed";
  client_id: string;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}
```

`services/frontend/apps/admin/src/features/jobs/queries.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Job, ListResponse } from "./types";

export function useJobs() {
  return useQuery({
    queryKey: ["jobs"],
    queryFn: () => apiClient.get<ListResponse<Job>>("/api/v1/jobs"),
  });
}
```

`services/frontend/apps/admin/src/features/jobs/job-list.tsx`:

```typescript
import { useJobs } from "./queries";
import type { Job } from "./types";

const STATUS_BADGE: Record<Job["status"], string> = {
  open: "bg-success-bg text-success",
  draft: "bg-background-muted text-foreground-secondary",
  in_progress: "bg-warning-bg text-warning",
  closed: "bg-background-muted text-foreground-secondary",
};

function formatBudget(amount: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(amount / 100);
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const days = Math.floor(diff / 86_400_000);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}

export function JobList() {
  const { data, isLoading, isError, error } = useJobs();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading jobs...
      </div>
    );
  }

  if (isError) {
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const jobs = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Jobs</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {jobs.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No jobs found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Title</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Client</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Budget</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Status</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {jobs.map((job) => (
                <tr key={job.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-medium text-foreground">{job.title}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{job.client_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBudget(job.budget_min)} &ndash; {formatBudget(job.budget_max)}</td>
                  <td className="px-6 py-4">
                    <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE[job.status]}`}>
                      {job.status.replace("_", " ")}
                    </span>
                  </td>
                  <td className="px-6 py-4 text-foreground-secondary">{timeAgo(job.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
```

Note: admin view uses a table layout (not cards) for data density. No "New Job" button — admin is read-only.

**Step 3: Contracts feature**

`services/frontend/apps/admin/src/features/contracts/types.ts`:

```typescript
export interface Contract {
  id: string;
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  status: string;
  client_wallet_id: string;
  freelancer_wallet_id: string;
  hold_id?: string;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}
```

`services/frontend/apps/admin/src/features/contracts/queries.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Contract, ListResponse } from "./types";

export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: () => apiClient.get<ListResponse<Contract>>("/api/v1/contracts"),
  });
}
```

`services/frontend/apps/admin/src/features/contracts/contract-list.tsx`:

```typescript
import { useContracts } from "./queries";

const STATUS_BADGE: Record<string, string> = {
  PENDING: "bg-background-muted text-foreground-secondary",
  HOLD_PENDING: "bg-warning-bg text-warning",
  AWAITING_ACCEPT: "bg-info-bg text-info",
  ACTIVE: "bg-success-bg text-success",
  COMPLETING: "bg-warning-bg text-warning",
  COMPLETED: "bg-success-bg text-success",
  DECLINING: "bg-error-bg text-error",
  DECLINED: "bg-background-muted text-foreground-secondary",
  CANCELLED: "bg-background-muted text-foreground-secondary",
};

function formatAmount(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}

export function ContractList() {
  const { data, isLoading, isError, error } = useContracts();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading contracts...
      </div>
    );
  }

  if (isError) {
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const contracts = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Contracts</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {contracts.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No contracts found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Title</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Client</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Freelancer</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Amount</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {contracts.map((c) => (
                <tr key={c.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-medium text-foreground">{c.title}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{c.client_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{c.freelancer_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatAmount(c.amount)}</td>
                  <td className="px-6 py-4">
                    <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE[c.status] || "bg-background-muted text-foreground-secondary"}`}>
                      {c.status.replace("_", " ")}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
```

**Step 4: Wallets feature**

`services/frontend/apps/admin/src/features/wallets/types.ts`:

```typescript
export interface Wallet {
  id: string;
  user_id: string;
  balance: number;
  currency: string;
  available_balance: number;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}
```

`services/frontend/apps/admin/src/features/wallets/queries.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Wallet, ListResponse } from "./types";

export function useWallets() {
  return useQuery({
    queryKey: ["wallets"],
    queryFn: () => apiClient.get<ListResponse<Wallet>>("/api/v1/wallets"),
  });
}
```

`services/frontend/apps/admin/src/features/wallets/wallet-list.tsx`:

```typescript
import { useWallets } from "./queries";

function formatBalance(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}

export function WalletList() {
  const { data, isLoading, isError, error } = useWallets();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading wallets...
      </div>
    );
  }

  if (isError) {
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const wallets = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Wallets</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {wallets.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No wallets found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">User</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Balance</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Available</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Currency</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {wallets.map((w) => (
                <tr key={w.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{w.user_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBalance(w.balance)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBalance(w.available_balance)}</td>
                  <td className="px-6 py-4 text-foreground-secondary">{w.currency}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
```

**Step 5: Create route files**

`services/frontend/apps/admin/src/routes/_auth/index.tsx`:

```typescript
import { createFileRoute } from "@tanstack/react-router";
import { DashboardPage } from "@/features/dashboard/dashboard-page";

export const Route = createFileRoute("/_auth/")({
  component: DashboardPage,
});
```

`services/frontend/apps/admin/src/routes/_auth/jobs.index.tsx`:

```typescript
import { createFileRoute } from "@tanstack/react-router";
import { JobList } from "@/features/jobs/job-list";

export const Route = createFileRoute("/_auth/jobs/")({
  component: JobList,
});
```

`services/frontend/apps/admin/src/routes/_auth/contracts.index.tsx`:

```typescript
import { createFileRoute } from "@tanstack/react-router";
import { ContractList } from "@/features/contracts/contract-list";

export const Route = createFileRoute("/_auth/contracts/")({
  component: ContractList,
});
```

`services/frontend/apps/admin/src/routes/_auth/wallets.index.tsx`:

```typescript
import { createFileRoute } from "@tanstack/react-router";
import { WalletList } from "@/features/wallets/wallet-list";

export const Route = createFileRoute("/_auth/wallets/")({
  component: WalletList,
});
```

**Step 6: Verify build**

Run: `cd services/frontend/apps/admin && npx tsc -b && npx vite build`
Expected: Build succeeds

**Step 7: Commit**

```bash
git add services/frontend/apps/admin/src/
git commit -m "feat(frontend-admin): add dashboard, jobs, contracts, wallets pages"
```

---

## Phase 6: Update Existing Frontends for `{items, total}` Format

### Task 11: Update client frontend queries for new response format

**Files:**
- Modify: `services/frontend/apps/client/src/features/jobs/queries.ts:5-9`
- Modify: `services/frontend/apps/client/src/features/jobs/types.ts` (add ListResponse)
- Modify: `services/frontend/apps/client/src/features/jobs/job-list.tsx:29,62,68`
- Modify: `services/frontend/apps/client/src/features/contracts/queries.ts:5-9`
- Modify: `services/frontend/apps/client/src/features/contracts/types.ts` (add ListResponse)

**Step 1: Update jobs types**

In `services/frontend/apps/client/src/features/jobs/types.ts`, add at the end:

```typescript
export interface ListResponse<T> {
  items: T[];
  total: number;
}
```

**Step 2: Update jobs query**

In `services/frontend/apps/client/src/features/jobs/queries.ts`, change line 8:

```typescript
// FROM:
queryFn: () => apiClient.get<Job[]>("/api/v1/jobs"),
// TO:
queryFn: () => apiClient.get<ListResponse<Job>>("/api/v1/jobs"),
```

Add the import: `import type { Job, CreateJobRequest, ListResponse } from "./types";`

**Step 3: Update job-list.tsx to read from .items**

In `services/frontend/apps/client/src/features/jobs/job-list.tsx`:

- Line 29: Change `const { data: jobs, ...` to `const { data, ...`
- Line 62: Change `{!jobs || jobs.length === 0 ?` to `{!data?.items?.length ?`
- Line 68: Change `{jobs.map((job) =>` to `{data!.items.map((job) =>`

**Step 4: Update contracts types**

In `services/frontend/apps/client/src/features/contracts/types.ts`, add at the end:

```typescript
export interface ListResponse<T> {
  items: T[];
  total: number;
}
```

**Step 5: Update contracts query**

In `services/frontend/apps/client/src/features/contracts/queries.ts`, change line 8:

```typescript
// FROM:
queryFn: () => apiClient.get<Contract[]>("/api/v1/contracts"),
// TO:
queryFn: () => apiClient.get<ListResponse<Contract>>("/api/v1/contracts"),
```

Add the import: `import type { Contract, CreateContractRequest, ListResponse } from "./types";`

**Step 6: Update any contract list components that read the data**

Find components that use `useContracts()` and update to read `.items`.

**Step 7: Run existing client tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: Tests may need updating for new response shape

**Step 8: Commit**

```bash
git add services/frontend/apps/client/src/features/
git commit -m "fix(frontend-client): update queries for {items, total} response format"
```

---

### Task 12: Update freelancer frontend queries for new response format

**Files:**
- Modify: `services/frontend/apps/freelancer/src/features/contracts/queries.ts`
- Modify: `services/frontend/apps/freelancer/src/features/contracts/types.ts` (if separate)

**Step 1: Apply the same `{items, total}` migration as Task 11**

Same pattern: update `useContracts()` to expect `ListResponse<Contract>`, update components to read `.items`.

Note: wallet queries (`useWallet()`) return a single object, not a list — no change needed.

**Step 2: Run freelancer tests**

Run: `cd services/frontend/apps/freelancer && pnpm test`
Expected: PASS

**Step 3: Commit**

```bash
git add services/frontend/apps/freelancer/src/features/
git commit -m "fix(frontend-freelancer): update queries for {items, total} response format"
```

---

## Phase 7: TODOS + CLAUDE.md Updates

### Task 13: Update TODOS.md and CLAUDE.md

**Files:**
- Modify: `TODOS.md` (add Playwright e2e for admin)
- Modify: `CLAUDE.md` (add :5175 to port table)

**Step 1: Add Playwright TODO**

In `TODOS.md`, update the existing Playwright TODO to include admin:

```markdown
### Add Playwright e2e tests for client and admin frontends
- **What:** End-to-end browser tests for client (login → create job → view matches → propose contract) and admin (login → dashboard → jobs → contracts → wallets) flows
- **Why:** Vitest + RTL tests mock the API. E2e tests verify the real stack works together (CORS, cookies, Traefik routing, role enforcement)
- **Context:** M8/M10 ship with Vitest + RTL + msw. E2e tests require Docker Compose running. Can be added post-M10.
- **Depends on:** M10 complete
```

**Step 2: Add frontend-admin port to CLAUDE.md**

In `CLAUDE.md`, add to the Service Ports table:

```
| frontend-admin     | 5175 | Vite      |
```

**Step 3: Commit**

```bash
git add TODOS.md CLAUDE.md
git commit -m "docs: add admin frontend port, update Playwright e2e TODO for admin"
```

---

## Merge Criteria Verification Checklist

After all tasks are complete, verify:

1. `make up` — all services start healthy including rebuilt bff-admin
2. `make health` — all services respond to /health
3. Admin login at http://localhost:5175/login with admin@example.com / password
4. Dashboard shows stats from all 3 services
5. Jobs page shows all jobs (not filtered by user)
6. Contracts page shows all contracts (not filtered by user)
7. Wallets page shows all wallets
8. Client frontend at :5173 still works (login, jobs, contracts)
9. Freelancer frontend at :5174 still works (login, contracts)
10. `cd services/bff/admin && go test ./cmd/server/ -v` — all tests pass
11. `cd pkg/bff && go test -v -run TestRequireRole` — RequireRole tests pass
12. `cd services/frontend/apps/admin && pnpm test` — frontend tests pass
