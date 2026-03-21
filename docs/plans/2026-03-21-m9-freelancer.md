# M9: Freelancer BFF + Freelancer Frontend + Client Redesign — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship a modern freelancer-facing app (login, view job matches, accept contracts, check wallet) alongside a redesigned client app (top nav, card layouts, new design tokens).

**Architecture:** Three independent workstreams executed sequentially. Shared code is extracted first (pkg/bff for Go, packages/ui for React) so both BFFs and both frontends import from a single source. Backend gets one new endpoint (contracts list with filtering). Freelancer frontend is a separate Vite app at :5174.

**Tech Stack:** Go 1.25 stdlib, React 19, TanStack Router + Query, Tailwind CSS v4, Vitest + RTL + MSW

**Eng Review Decisions (2026-03-21):**
1. Extract shared BFF code to `pkg/bff/` (DRY across 3 BFFs)
2. Contracts list: `GET /api/v1/contracts?freelancer_id=X&client_id=X` with backend filtering
3. Freelancer frontend: separate Vite app `apps/freelancer/` at `:5174`
4. Shared auth + api-client in `packages/ui/`
5. Shared TopNav component in `packages/ui/`
6. Tests: `pkg/bff/` unit tests + per-BFF smoke tests + full frontend coverage

---

## Phase 1: Backend — Shared BFF Package + Contracts List Endpoint

### Task 1: Extract shared BFF code to pkg/bff/

This task moves auth, middleware, and service client code from `services/bff/client/cmd/server/` into a reusable `pkg/bff/` package. Both bff-client and bff-freelancer will import from it.

**Files:**
- Create: `pkg/bff/go.mod`
- Create: `pkg/bff/context.go`
- Create: `pkg/bff/auth.go`
- Create: `pkg/bff/client.go`
- Create: `pkg/bff/http.go`
- Create: `pkg/bff/middleware.go`
- Create: `pkg/bff/auth_test.go`
- Create: `pkg/bff/middleware_test.go`
- Create: `pkg/bff/client_test.go`
- Modify: `services/bff/client/cmd/server/main.go`
- Modify: `services/bff/client/go.mod`
- Modify: `services/bff/freelancer/go.mod`
- Modify: `go.work`

**Step 1: Create pkg/bff/go.mod**

```
module github.com/copycatsh/hire-flow/pkg/bff

go 1.25.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/stretchr/testify v1.11.1
	golang.org/x/time v0.15.0
)
```

Run: `cd pkg/bff && go mod tidy`

**Step 2: Create pkg/bff/context.go**

Context keys and helpers shared by all BFF code:

```go
package bff

import "context"

type contextKey string

const (
	CtxKeyUserID contextKey = "user_id"
	CtxKeyRole   contextKey = "role"
)

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, CtxKeyUserID, userID)
}

func UserIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(CtxKeyUserID).(string)
	return s
}

func RoleFrom(ctx context.Context) string {
	s, _ := ctx.Value(CtxKeyRole).(string)
	return s
}
```

**Step 3: Create pkg/bff/http.go**

JSON response helpers:

```go
package bff

import (
	"encoding/json"
	"net/http"
)

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

**Step 4: Create pkg/bff/auth.go**

Move `AuthConfig`, `GenerateTokens`, `JWTMiddleware`, `AuthHandler` (login/refresh), and `setTokenCookies` from bff-client. Key changes: package name `bff`, exported names, use `CtxKeyUserID`/`CtxKeyRole`.

The `TestUsers` map stays in this package (it's shared test data for dev).

```go
package bff

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var TestUsers = map[string]struct {
	Password string
	UserID   string
	Role     string
}{
	"client@example.com":     {Password: "password", UserID: "11111111-1111-1111-1111-111111111111", Role: "client"},
	"client2@example.com":    {Password: "password", UserID: "22222222-2222-2222-2222-222222222222", Role: "client"},
	"freelancer@example.com": {Password: "password", UserID: "33333333-3333-3333-3333-333333333333", Role: "freelancer"},
	"admin@example.com":      {Password: "password", UserID: "44444444-4444-4444-4444-444444444444", Role: "admin"},
}

type AuthConfig struct {
	Secret          []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CookieSecure    bool
}

func (a *AuthConfig) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "missing access token")
			return
		}

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return a.Secret, nil
		})
		if err != nil || !token.Valid {
			WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			WriteError(w, http.StatusUnauthorized, "invalid claims")
			return
		}

		tokenType, _ := claims["type"].(string)
		if tokenType != "access" {
			WriteError(w, http.StatusUnauthorized, "invalid token type")
			return
		}

		sub, _ := claims["sub"].(string)
		role, _ := claims["role"].(string)
		if sub == "" || role == "" {
			WriteError(w, http.StatusUnauthorized, "missing required claims")
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyUserID, sub)
		ctx = context.WithValue(ctx, CtxKeyRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *AuthConfig) GenerateTokens(userID, role string) (accessToken, refreshToken string, err error) {
	now := time.Now()

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"exp":  now.Add(a.AccessTokenTTL).Unix(),
		"iat":  now.Unix(),
		"type": "access",
	})
	accessToken, err = access.SignedString(a.Secret)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}

	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"exp":  now.Add(a.RefreshTokenTTL).Unix(),
		"iat":  now.Unix(),
		"type": "refresh",
	})
	refreshToken, err = refresh.SignedString(a.Secret)
	if err != nil {
		return "", "", fmt.Errorf("sign refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

type AuthHandler struct {
	Auth *AuthConfig
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.Login)
	mux.HandleFunc("POST /auth/refresh", h.Refresh)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, ok := TestUsers[req.Email]
	if !ok || user.Password != req.Password {
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, refreshToken, err := h.Auth.GenerateTokens(user.UserID, user.Role)
	if err != nil {
		slog.Error("failed to generate tokens", "user_id", user.UserID, "error", err)
		WriteError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	h.setTokenCookies(w, accessToken, refreshToken)
	WriteJSON(w, http.StatusOK, map[string]string{
		"user_id": user.UserID,
		"role":    user.Role,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.Auth.Secret, nil
	})
	if err != nil || !token.Valid {
		WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "invalid claims")
		return
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		WriteError(w, http.StatusUnauthorized, "invalid token type")
		return
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	if sub == "" || role == "" {
		WriteError(w, http.StatusUnauthorized, "missing required claims")
		return
	}

	accessToken, refreshToken, err := h.Auth.GenerateTokens(sub, role)
	if err != nil {
		slog.Error("failed to generate tokens", "user_id", sub, "error", err)
		WriteError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	h.setTokenCookies(w, accessToken, refreshToken)
	WriteJSON(w, http.StatusOK, map[string]string{
		"user_id": sub,
		"role":    role,
	})
}

func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	sameSite := http.SameSiteStrictMode
	if !h.Auth.CookieSecure {
		sameSite = http.SameSiteLaxMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Auth.CookieSecure,
		SameSite: sameSite,
		MaxAge:   int(h.Auth.AccessTokenTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/auth/refresh",
		HttpOnly: true,
		Secure:   h.Auth.CookieSecure,
		SameSite: sameSite,
		MaxAge:   int(h.Auth.RefreshTokenTTL.Seconds()),
	})
}
```

Note: `context` import needed in `JWTMiddleware`. Add to the import block.

**Step 5: Create pkg/bff/client.go**

Move `ServiceClient`, `APIError`, `Do`, `Forward` from bff-client. Use exported context helpers.

```go
package bff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type APIError struct {
	StatusCode int
	Body       string
	Service    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Service, e.StatusCode, e.Body)
}

type ServiceClient struct {
	BaseURL string
	HTTP    *http.Client
	Name    string
}

func (c *ServiceClient) Do(ctx context.Context, method, path string, body any, dest any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if userID := UserIDFrom(ctx); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", c.Name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Service:    c.Name,
		}
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *ServiceClient) Forward(ctx context.Context, w http.ResponseWriter, method, path string, body io.Reader) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create upstream request", "service", c.Name, "error", err)
		WriteError(w, http.StatusBadGateway, fmt.Sprintf("%s: service unavailable", c.Name))
		return
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID := UserIDFrom(ctx); userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "upstream request failed", "service", c.Name, "method", method, "path", path, "error", err)
		WriteError(w, http.StatusBadGateway, fmt.Sprintf("%s: service unavailable", c.Name))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.ErrorContext(ctx, "forwarding response body", "service", c.Name, "error", err)
	}
}
```

**Step 6: Create pkg/bff/middleware.go**

Move `RateLimiter`, `RequestLogger`, `statusWriter` from bff-client.

```go
package bff

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
}

func (rl *RateLimiter) getLimiter(userID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, ok := rl.limiters[userID]
	if !ok {
		l = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[userID] = l
	}
	return l
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFrom(r.Context())
		if userID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !rl.getLimiter(userID).Allow() {
			WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}
```

**Step 7: Write pkg/bff/ tests**

Port existing tests from bff-client. Test files: `auth_test.go`, `middleware_test.go`, `client_test.go`. Copy the test patterns from `services/bff/client/cmd/server/*_test.go` but update imports to use `bff` package.

Run: `cd pkg/bff && go test ./... -v`
Expected: All tests PASS

**Step 8: Refactor bff-client to import pkg/bff/**

Update `services/bff/client/go.mod`:
- Add `require github.com/copycatsh/hire-flow/pkg/bff v0.0.0-00010101000000-000000000000`
- Add `replace github.com/copycatsh/hire-flow/pkg/bff => ../../../pkg/bff`

Update `services/bff/client/cmd/server/main.go`:
- Import `"github.com/copycatsh/hire-flow/pkg/bff"`
- Replace `AuthConfig` → `bff.AuthConfig`
- Replace `NewRateLimiter` → `bff.NewRateLimiter`
- Replace `ServiceClient` → `bff.ServiceClient`
- Replace `AuthHandler` → `bff.AuthHandler`
- Replace `RequestLogger` → `bff.RequestLogger`

Delete from bff-client (now in pkg/bff):
- `auth_handler.go`, `middleware.go`, `client.go`
- Their test files (tests are now in pkg/bff)

Keep in bff-client (handler files use `bff.ServiceClient`, `bff.CtxKeyUserID`, etc.):
- `job_handler.go`, `match_handler.go`, `contract_handler.go`, `payment_handler.go`
- `handler_test.go`, `integration_test.go`

Update handler files to use `bff.UserIDFrom(ctx)` instead of `ctx.Value(ctxKeyUserID)` and `bff.WriteError`, `bff.WriteJSON` etc.

Run: `cd services/bff/client && go test ./cmd/server/... -v`
Expected: All tests PASS

**Step 9: Add pkg/bff to go.work**

```
use (
	./pkg/bff
	./pkg/outbox
	...
)
```

Run: `cd /Users/anton/WorkProjects/pet_projects/hire-flow && go work sync`

**Step 10: Commit**

```bash
git add pkg/bff/ services/bff/client/ go.work
git commit -m "refactor(m9): extract shared BFF code to pkg/bff

Moves auth handler, JWT middleware, rate limiter, service client,
and HTTP helpers from bff-client into a shared package.
Both bff-client and bff-freelancer will import from it."
```

---

### Task 2: Add contracts list endpoint

**Files:**
- Modify: `services/contracts/cmd/server/contract.go` (add `List` to interface)
- Modify: `services/contracts/cmd/server/contract_store.go` (add `List` query)
- Modify: `services/contracts/cmd/server/handler.go` (add `ListContracts` handler + route)
- Create: `services/contracts/cmd/server/contract_store_list_test.go`
- Modify: `services/contracts/cmd/server/handler_test.go` (add list handler tests)

**Step 1: Add List to ContractStore interface**

In `contract.go`, add to `ContractStore` interface:

```go
List(ctx context.Context, db DBTX, filter ListFilter) ([]Contract, error)
```

Add the filter type:

```go
type ListFilter struct {
	ClientID     string
	FreelancerID string
	Limit        int
	Offset       int
}
```

**Step 2: Implement List in MySQLContractStore**

In `contract_store.go`:

```go
func (s *MySQLContractStore) List(ctx context.Context, db DBTX, filter ListFilter) ([]Contract, error) {
	query := `SELECT id, client_id, freelancer_id, title, description, amount, currency, status, client_wallet_id, freelancer_wallet_id, hold_id, created_at, updated_at FROM contracts WHERE 1=1`
	args := []any{}

	if filter.ClientID != "" {
		query += " AND client_id = ?"
		args = append(args, filter.ClientID)
	}
	if filter.FreelancerID != "" {
		query += " AND freelancer_id = ?"
		args = append(args, filter.FreelancerID)
	}

	query += " ORDER BY created_at DESC"

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("contract list: %w", err)
	}
	defer rows.Close()

	var contracts []Contract
	for rows.Next() {
		var c Contract
		if err := rows.Scan(&c.ID, &c.ClientID, &c.FreelancerID, &c.Title, &c.Description, &c.Amount, &c.Currency, &c.Status, &c.ClientWalletID, &c.FreelancerWalletID, &c.HoldID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("contract list scan: %w", err)
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}
```

**Step 3: Add ListContracts handler**

In `handler.go`, add route to `RegisterRoutes`:

```go
r.Get("/", h.ListContracts)
```

Add handler:

```go
func (h *ContractHandler) ListContracts(w http.ResponseWriter, r *http.Request) {
	filter := ListFilter{
		ClientID:     r.URL.Query().Get("client_id"),
		FreelancerID: r.URL.Query().Get("freelancer_id"),
	}

	if filter.ClientID == "" && filter.FreelancerID == "" {
		http.Error(w, `{"error":"client_id or freelancer_id required"}`, http.StatusBadRequest)
		return
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &filter.Limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &filter.Offset)
	}

	contracts, err := h.contracts.List(r.Context(), h.db, filter)
	if err != nil {
		slog.ErrorContext(r.Context(), "list contracts", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if contracts == nil {
		contracts = []Contract{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contracts)
}
```

**Step 4: Write tests**

Test the store List method (table-driven: filter by client_id, filter by freelancer_id, pagination, empty result). Test the handler (valid request, missing filter → 400).

Run: `cd services/contracts && go test ./cmd/server/... -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add services/contracts/
git commit -m "feat(m9): add GET /api/v1/contracts list endpoint with filtering

Supports ?client_id=X and ?freelancer_id=X query params with
limit/offset pagination. Required for both client and freelancer
contracts list pages."
```

---

### Task 3: Build BFF Freelancer

**Files:**
- Modify: `services/bff/freelancer/go.mod` (add pkg/bff dependency)
- Rewrite: `services/bff/freelancer/cmd/server/main.go`
- Create: `services/bff/freelancer/cmd/server/match_handler.go`
- Create: `services/bff/freelancer/cmd/server/contract_handler.go`
- Create: `services/bff/freelancer/cmd/server/payment_handler.go`
- Create: `services/bff/freelancer/cmd/server/handler_test.go`
- Modify: `compose.yaml` (add env vars + depends_on for bff-freelancer)
- Modify: `infra/traefik/dynamic.yml` (add CORS origin for :5174)

**Step 1: Update bff-freelancer go.mod**

Add to requires:
```
github.com/copycatsh/hire-flow/pkg/bff v0.0.0-00010101000000-000000000000
```

Add replace:
```
replace github.com/copycatsh/hire-flow/pkg/bff => ../../../pkg/bff
```

Run: `cd services/bff/freelancer && go mod tidy`

**Step 2: Rewrite main.go**

Follow bff-client pattern exactly. Key differences:
- Service name: `"bff-freelancer"`
- Port: `8011`
- No `JobHandler.Create` — freelancers don't create jobs
- Adds `MatchHandler` that calls `POST /api/v1/match/profile/{profile_id}` (not job-based matching)
- ContractHandler includes `List` (GET with query forwarding) + `GetByID` + `Accept` (no Create/Complete/Cancel)

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
```

**Step 3: Create match_handler.go**

Freelancer matching: find jobs that match the freelancer's profile.

```go
package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type MatchHandler struct {
	matching *bff.ServiceClient
}

func (h *MatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/matches", h.FindJobMatches)
}

func (h *MatchHandler) FindJobMatches(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profile_id")
	if profileID == "" {
		bff.WriteError(w, http.StatusBadRequest, "profile_id query param required")
		return
	}
	path := "/api/v1/match/profile/" + url.PathEscape(profileID)
	if topK := r.URL.Query().Get("top_k"); topK != "" {
		path += "?top_k=" + url.QueryEscape(topK)
	}
	h.matching.Forward(r.Context(), w, http.MethodPost, path, r.Body)
}
```

**Step 4: Create contract_handler.go**

```go
package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type ContractHandler struct {
	contracts *bff.ServiceClient
}

func (h *ContractHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/contracts", h.List)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/contracts/{id}/accept", h.Accept)
}

func (h *ContractHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := bff.UserIDFrom(r.Context())
	if userID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	path := "/api/v1/contracts?freelancer_id=" + url.QueryEscape(userID)
	h.contracts.Forward(r.Context(), w, http.MethodGet, path, nil)
}

func (h *ContractHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	h.contracts.Forward(r.Context(), w, http.MethodGet, "/api/v1/contracts/"+id, nil)
}

func (h *ContractHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id := url.PathEscape(r.PathValue("id"))
	h.contracts.Forward(r.Context(), w, http.MethodPut, "/api/v1/contracts/"+id+"/accept", r.Body)
}
```

**Step 5: Create payment_handler.go**

```go
package main

import (
	"net/http"
	"net/url"

	"github.com/copycatsh/hire-flow/pkg/bff"
)

type PaymentHandler struct {
	payments *bff.ServiceClient
}

func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallet", h.GetBalance)
}

func (h *PaymentHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID := bff.UserIDFrom(r.Context())
	if userID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, "/api/v1/payments/wallet/"+url.PathEscape(userID), nil)
}
```

**Step 6: Write handler smoke tests**

Table-driven tests verifying each route is registered and proxies correctly (mock backend with httptest.Server). Cover: matches endpoint, contracts list/get/accept, wallet.

Run: `cd services/bff/freelancer && go test ./cmd/server/... -v`
Expected: All tests PASS

**Step 7: Update compose.yaml**

Add env vars to `bff-freelancer` service:

```yaml
bff-freelancer:
  build:
    context: .
    dockerfile: services/bff/freelancer/Dockerfile
  container_name: hire-flow-bff-freelancer
  environment:
    JWT_SECRET: "dev-secret-change-in-production"
    MATCHING_URL: "http://ai-matching:8002"
    CONTRACTS_URL: "http://contracts:8003"
    PAYMENTS_URL: "http://payments:8004"
    OTEL_ENDPOINT: "otel-collector:4317"
    COOKIE_SECURE: "false"
  ports:
    - "8011:8011"
  networks:
    - hire-flow
  depends_on:
    ai-matching:
      condition: service_healthy
    contracts:
      condition: service_healthy
    payments:
      condition: service_healthy
    otel-collector:
      condition: service_healthy
  healthcheck:
    test: ["CMD-SHELL", "wget -qO- http://localhost:8011/health > /dev/null 2>&1"]
    interval: 5s
    timeout: 3s
    retries: 5
```

**Step 8: Update Traefik CORS**

In `infra/traefik/dynamic.yml`, add freelancer Vite dev server origin:

```yaml
accessControlAllowOriginList:
  - "http://localhost:5173"
  - "http://localhost:5174"
```

**Step 9: Verify**

Run: `make up && make health`
Expected: All services healthy, including bff-freelancer

Run: `curl -s http://localhost:8011/health | jq .`
Expected: `{"status":"ok"}`

**Step 10: Commit**

```bash
git add services/bff/freelancer/ compose.yaml infra/traefik/dynamic.yml
git commit -m "feat(m9): implement BFF Freelancer with matches, contracts, wallet

Freelancer BFF at :8011 proxies to ai-matching (profile→job matches),
contracts (list/get/accept), and payments (wallet balance).
Uses shared pkg/bff for auth, middleware, and service client."
```

---

## Phase 2: Frontend — Shared Components + Client Redesign

### Task 4: Extract shared frontend code to packages/ui/

**Files:**
- Modify: `packages/ui/package.json` (add react peer deps, add exports)
- Modify: `packages/ui/src/index.ts` (add exports)
- Create: `packages/ui/src/auth/auth-context.tsx`
- Create: `packages/ui/src/auth/types.ts`
- Create: `packages/ui/src/auth/use-login.ts`
- Create: `packages/ui/src/lib/create-api-client.ts`
- Create: `packages/ui/src/components/top-nav.tsx`
- Modify: `apps/client/src/features/auth/auth-context.tsx` (re-export from ui)
- Modify: `apps/client/src/lib/api-client.ts` (use factory)

**Step 1: Create packages/ui/src/lib/create-api-client.ts**

Configurable api-client factory — takes `baseUrl` param:

```typescript
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function createApiClient(baseUrl: string) {
  let refreshPromise: Promise<void> | null = null;

  async function refreshToken(): Promise<void> {
    const res = await fetch(`${baseUrl}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      throw new ApiError(res.status, "refresh failed");
    }
  }

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const opts: RequestInit = {
      method,
      credentials: "include",
      headers: {} as Record<string, string>,
    };

    if (body !== undefined) {
      (opts.headers as Record<string, string>)["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }

    let res = await fetch(`${baseUrl}${path}`, opts);

    if (res.status === 401) {
      try {
        if (!refreshPromise) {
          refreshPromise = refreshToken();
        }
        await refreshPromise;
        refreshPromise = null;
        res = await fetch(`${baseUrl}${path}`, opts);
      } catch {
        refreshPromise = null;
        window.location.assign("/login");
        throw new ApiError(401, "session expired");
      }
    }

    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: "request failed" }));
      throw new ApiError(res.status, data.error || `HTTP ${res.status}`);
    }

    return res.json() as Promise<T>;
  }

  return {
    get: <T>(path: string) => request<T>("GET", path),
    post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
    put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  };
}
```

**Step 2: Move auth code to packages/ui/src/auth/**

Copy `auth-context.tsx`, `types.ts` from client app to `packages/ui/src/auth/`. These are identical for both apps.

Create `packages/ui/src/auth/use-login.ts`:

```typescript
import { useMutation } from "@tanstack/react-query";
import { useAuth } from "./auth-context";

interface LoginResponse {
  user_id: string;
  role: string;
}

export function useLogin(apiClient: { post: <T>(path: string, body?: unknown) => Promise<T> }) {
  const { setUser } = useAuth();

  return useMutation({
    mutationFn: (data: { email: string; password: string }) =>
      apiClient.post<LoginResponse>("/auth/login", data),
    onSuccess: (data) => {
      setUser(data);
    },
  });
}
```

**Step 3: Create packages/ui/src/components/top-nav.tsx**

Shared top navigation component with configurable nav items:

```tsx
import { type ReactNode } from "react";
import { cn } from "../lib/utils";

export interface NavItem {
  to: string;
  label: string;
  icon: ReactNode;
}

interface TopNavProps {
  appName: string;
  navItems: NavItem[];
  currentPath: string;
  avatar?: string;
  onLogout?: () => void;
  renderLink: (props: { to: string; className: string; children: ReactNode }) => ReactNode;
}

export function TopNav({ appName, navItems, currentPath, avatar, onLogout, renderLink }: TopNavProps) {
  return (
    <nav className="sticky top-0 z-50 flex h-16 items-center justify-between border-b border-border bg-background px-6">
      <div className="flex items-center gap-8">
        <span className="font-display text-lg font-bold tracking-tight">
          hire<span className="text-primary">{appName}</span>
        </span>
        <div className="flex items-center gap-1">
          {navItems.map((item) => (
            <span key={item.label}>
              {renderLink({
                to: item.to,
                className: cn(
                  "flex items-center gap-1.5 rounded-sm px-3 py-2 text-sm font-medium text-foreground-secondary transition-colors",
                  "hover:bg-background-muted hover:text-foreground",
                  currentPath.startsWith(item.to) && "bg-primary-light text-primary",
                ),
                children: (
                  <>
                    {item.icon}
                    {item.label}
                  </>
                ),
              })}
            </span>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-3">
        {onLogout && (
          <button
            onClick={onLogout}
            className="rounded-sm px-3 py-1.5 text-sm text-foreground-secondary transition-colors hover:bg-background-muted hover:text-foreground"
          >
            Logout
          </button>
        )}
        {avatar && (
          <div className="flex h-8 w-8 items-center justify-content-center rounded-full bg-gradient-to-br from-primary to-accent text-xs font-semibold text-white">
            {avatar}
          </div>
        )}
      </div>
    </nav>
  );
}
```

**Step 4: Update packages/ui/src/index.ts exports**

```typescript
export { cn } from "./lib/utils";
export { createApiClient, ApiError } from "./lib/create-api-client";
export { AuthProvider, useAuth } from "./auth/auth-context";
export { useLogin } from "./auth/use-login";
export type { AuthUser } from "./auth/types";
export { TopNav } from "./components/top-nav";
export type { NavItem } from "./components/top-nav";
```

**Step 5: Update packages/ui/package.json**

Add `@tanstack/react-query` to `peerDependencies`.

**Step 6: Refactor client app to use shared code**

- `apps/client/src/lib/api-client.ts` → `import { createApiClient } from "@hire-flow/ui"; export const apiClient = createApiClient("/client");`
- `apps/client/src/features/auth/auth-context.tsx` → `export { AuthProvider, useAuth } from "@hire-flow/ui";`
- `apps/client/src/features/auth/types.ts` → `export type { AuthUser } from "@hire-flow/ui";`
- Update login page to use `useLogin` from `@hire-flow/ui`
- Delete old duplicated code files

Run: `cd services/frontend && pnpm install && pnpm --filter @hire-flow/client test`
Expected: All tests PASS

**Step 7: Commit**

```bash
git add services/frontend/packages/ services/frontend/apps/client/
git commit -m "refactor(m9): extract shared auth, api-client, TopNav to packages/ui

Both client and freelancer apps import auth context, login hook,
api-client factory, and TopNav component from @hire-flow/ui."
```

---

### Task 5: Redesign client app

Apply new DESIGN.md: top nav replaces sidebar, tables become cards, new tokens applied.

**Files:**
- Modify: `apps/client/src/routes/_auth.tsx` (sidebar → top nav layout)
- Delete: `apps/client/src/features/layout/sidebar.tsx`
- Modify: `apps/client/src/features/jobs/job-list.tsx` (table → cards)
- Modify: `apps/client/src/features/matches/match-list.tsx` (apply new design)
- Modify: `apps/client/src/features/wallet/wallet-page.tsx` (apply new design)
- Modify: `apps/client/src/features/contracts/contract-detail.tsx` (apply new design)
- Create: `apps/client/src/features/contracts/contract-list.tsx`
- Create: `apps/client/src/features/contracts/queries.ts` (add useContracts list query)
- Create: `apps/client/src/routes/_auth/contracts.index.tsx` (list route — currently just form)
- Update: all test files for changed components

**Step 1: Replace auth layout with top nav**

`_auth.tsx` — replace sidebar with TopNav:

```tsx
import { createFileRoute, Outlet, useNavigate, Link, useRouterState } from "@tanstack/react-router";
import { useAuth, TopNav } from "@hire-flow/ui";
import { Briefcase, FileText, Users, Wallet } from "lucide-react";
import { useEffect } from "react";

const navItems = [
  { to: "/jobs", label: "Jobs", icon: <Briefcase size={16} /> },
  { to: "/contracts", label: "Contracts", icon: <FileText size={16} /> },
  { to: "/wallet", label: "Wallet", icon: <Wallet size={16} /> },
];

export const Route = createFileRoute("/_auth")({
  component: AuthLayout,
});

function AuthLayout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  useEffect(() => {
    if (!user) {
      navigate({ to: "/login" });
    }
  }, [user, navigate]);

  if (!user) return null;

  return (
    <div className="min-h-screen bg-background-subtle">
      <TopNav
        appName="flow"
        navItems={navItems}
        currentPath={pathname}
        avatar={user.user_id.slice(0, 2).toUpperCase()}
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

**Step 2: Delete sidebar.tsx**

Remove `apps/client/src/features/layout/sidebar.tsx` — no longer needed.

**Step 3: Redesign job-list.tsx with cards**

Replace the `<table>` with a card grid. Use new design tokens: `shadow-sm`, `rounded-md`, `hover:shadow-card-hover`, staggered entrance animation classes.

Key changes:
- Card grid: `grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3`
- Each card: `rounded-md border border-border bg-background p-6 shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-card-hover hover:border-primary-500`
- Status badges: rounded-full pills with dot indicator
- Budget in mono font at card footer
- Posted date as relative text

**Step 4: Create contracts list page**

New `contract-list.tsx` component + update contracts `queries.ts` to add `useContracts()` hook:

```typescript
export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: () => apiClient.get<Contract[]>("/api/v1/contracts?client_id=current"),
  });
}
```

Note: The BFF client needs a new `GET /api/v1/contracts` route that forwards with `?client_id={user_id}` — add this to bff-client `contract_handler.go`.

**Step 5: Update remaining components**

Apply new spacing, border-radius, shadows, and card patterns to:
- `wallet-page.tsx` — stat cards with shadows
- `contract-detail.tsx` — card layout with status badge
- `match-list.tsx` — match cards with amber accent scores

**Step 6: Update route for contracts index**

Update `routes/_auth/contracts.index.tsx` to render `ContractList` instead of `ProposeContractForm` (form moves to a separate route or modal).

**Step 7: Update tests**

Update component tests to match new DOM structure (cards instead of table rows). Add test for `ContractList`.

Run: `cd services/frontend && pnpm --filter @hire-flow/client test`
Expected: All tests PASS

**Step 8: Commit**

```bash
git add services/frontend/apps/client/ services/bff/client/
git commit -m "feat(m9): redesign client app — top nav, card layouts, new design tokens

Replaces dark sidebar with horizontal top nav. Job list uses cards
instead of table. Contracts list page added. All components use
Refined Modern design system from DESIGN.md."
```

---

## Phase 3: Freelancer Frontend App

### Task 6: Scaffold freelancer app

**Files:**
- Create: `apps/freelancer/package.json`
- Create: `apps/freelancer/index.html`
- Create: `apps/freelancer/vite.config.ts`
- Create: `apps/freelancer/vitest.config.ts`
- Create: `apps/freelancer/tsconfig.json`
- Create: `apps/freelancer/tsconfig.app.json`
- Create: `apps/freelancer/src/main.tsx`
- Create: `apps/freelancer/src/vite-env.d.ts`
- Create: `apps/freelancer/src/test/setup.ts`

**Step 1: Create package.json**

Copy from client, change name to `@hire-flow/freelancer`. Same dependencies.

**Step 2: Create index.html**

Same as client — Google Fonts CDN link, `<div id="root">`.

**Step 3: Create vite.config.ts**

Same as client but:
- Port: `5174`
- Proxy: `/freelancer` → `http://localhost:80`

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
    port: 5174,
    proxy: {
      "/freelancer": {
        target: "http://localhost:80",
        changeOrigin: true,
      },
    },
  },
});
```

**Step 4: Create tsconfig files**

Copy from client — identical configuration.

**Step 5: Create main.tsx**

Same pattern as client:

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";
import "@hire-flow/ui/globals.css";

const router = createRouter({ routeTree });

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

**Step 6: Create test setup**

Copy `test/setup.ts` from client.

**Step 7: Install dependencies**

Run: `cd services/frontend && pnpm install`

**Step 8: Commit**

```bash
git add services/frontend/apps/freelancer/
git commit -m "feat(m9): scaffold freelancer frontend app at :5174

Separate Vite app with TanStack Router, Tailwind CSS v4, and
shared @hire-flow/ui package. Same stack as client app."
```

---

### Task 7: Freelancer app — routes and features

**Files:**
- Create: `apps/freelancer/src/lib/api-client.ts`
- Create: `apps/freelancer/src/routes/__root.tsx`
- Create: `apps/freelancer/src/routes/index.tsx`
- Create: `apps/freelancer/src/routes/login.tsx`
- Create: `apps/freelancer/src/routes/_auth.tsx`
- Create: `apps/freelancer/src/routes/_auth/matches.tsx`
- Create: `apps/freelancer/src/routes/_auth/contracts.index.tsx`
- Create: `apps/freelancer/src/routes/_auth/contracts.$id.tsx`
- Create: `apps/freelancer/src/routes/_auth/wallet.tsx`
- Create: `apps/freelancer/src/features/matches/queries.ts`
- Create: `apps/freelancer/src/features/matches/types.ts`
- Create: `apps/freelancer/src/features/matches/match-list.tsx`
- Create: `apps/freelancer/src/features/contracts/queries.ts`
- Create: `apps/freelancer/src/features/contracts/types.ts`
- Create: `apps/freelancer/src/features/contracts/contract-list.tsx`
- Create: `apps/freelancer/src/features/contracts/contract-detail.tsx`
- Create: `apps/freelancer/src/features/wallet/queries.ts`
- Create: `apps/freelancer/src/features/wallet/types.ts`
- Create: `apps/freelancer/src/features/wallet/wallet-page.tsx`

**Step 1: Create api-client.ts**

```typescript
import { createApiClient } from "@hire-flow/ui";
export const apiClient = createApiClient("/freelancer");
```

**Step 2: Create __root.tsx**

Same as client — QueryClient + AuthProvider.

**Step 3: Create routes**

Follow client patterns but with freelancer-specific nav items:

- `index.tsx` — redirect to `/matches` or `/login`
- `login.tsx` — login page using `useLogin` from `@hire-flow/ui`
- `_auth.tsx` — TopNav with freelancer nav items:
  - Matches (Users icon)
  - Contracts (FileText icon)
  - Wallet (Wallet icon)

**Step 4: Create features/matches/**

`queries.ts`:
```typescript
import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { MatchResponse } from "./types";

export function useFindJobMatches(profileId: string) {
  return useMutation({
    mutationFn: () =>
      apiClient.post<MatchResponse>(`/api/v1/matches?profile_id=${profileId}`),
  });
}
```

`types.ts`:
```typescript
export interface JobMatch {
  id: string;
  score: number;
}

export interface MatchResponse {
  matches: JobMatch[];
  total: number;
}
```

`match-list.tsx` — card grid showing matched jobs with scores. Uses amber accent for match score badges. Cards with hover lift effect.

**Step 5: Create features/contracts/**

`queries.ts`:
```typescript
export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: () => apiClient.get<Contract[]>("/api/v1/contracts"),
  });
}

export function useContract(id: string) {
  return useQuery({
    queryKey: ["contracts", id],
    queryFn: () => apiClient.get<Contract>(`/api/v1/contracts/${id}`),
    enabled: !!id,
  });
}

export function useAcceptContract() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiClient.put<Contract>(`/api/v1/contracts/${id}/accept`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["contracts"] });
    },
  });
}
```

`contract-list.tsx` — card list of contracts with status badges. Contracts in `AWAITING_ACCEPT` show prominent "Accept" button.

`contract-detail.tsx` — full contract view with Accept/Decline actions.

**Step 6: Create features/wallet/**

Copy from client — same queries and component structure, but pointing to freelancer api-client.

**Step 7: Commit**

```bash
git add services/frontend/apps/freelancer/
git commit -m "feat(m9): implement freelancer frontend — matches, contracts, wallet

Freelancer app at :5174 with job match viewing, contract acceptance,
and wallet balance. Uses shared @hire-flow/ui components and
Refined Modern design system."
```

---

### Task 8: Frontend tests

**Files:**
- Create: `apps/freelancer/src/features/matches/matches.test.tsx`
- Create: `apps/freelancer/src/features/contracts/contracts.test.tsx`
- Create: `apps/freelancer/src/features/wallet/wallet.test.tsx`
- Modify: `apps/client/src/features/jobs/jobs.test.tsx` (update for card layout)
- Create: `packages/ui/src/components/top-nav.test.tsx`
- Create: `packages/ui/src/auth/auth-context.test.tsx` (move from client)

**Step 1: Write tests for each freelancer feature**

Follow existing test patterns: Vitest + RTL + MSW handlers.

Each test file should cover:
- Renders loading state
- Renders data when API returns successfully
- Renders empty state
- Renders error state
- User interactions (accept button for contracts)

**Step 2: Update client job tests for card layout**

Change assertions from table queries to card-based DOM queries.

**Step 3: Write TopNav tests**

Test: renders nav items, highlights active item, calls onLogout.

**Step 4: Run all tests**

Run: `cd services/frontend && pnpm --filter @hire-flow/client test && pnpm --filter @hire-flow/freelancer test`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add services/frontend/
git commit -m "test(m9): add full frontend test coverage for client and freelancer apps

Tests for TopNav, job cards, freelancer matches, contracts list,
contract accept, wallet. All using Vitest + RTL + MSW."
```

---

### Task 9: Update Tailwind source paths for freelancer

The `globals.css` in `packages/ui` has a `source()` directive pointing only to the client app. The freelancer app also needs to be scanned for Tailwind classes.

**Files:**
- Modify: `packages/ui/src/styles/globals.css`

**Step 1: Update source directive**

Change:
```css
@import "tailwindcss" source("../../../../apps/client/src");
```

To scan both apps. The freelancer app should have its own CSS import that includes the source for its directory. Alternatively, each app gets its own `@import "tailwindcss"` in their local CSS that imports `@hire-flow/ui/globals.css` for theme tokens only.

The cleanest approach: split `globals.css` into `tokens.css` (theme only, no source) and have each app's entry import tailwindcss with its own source + the shared tokens.

**Step 2: Verify Tailwind generates correct classes for both apps**

Run: `cd services/frontend && pnpm --filter @hire-flow/freelancer dev`
Expected: Styles render correctly

**Step 3: Commit**

```bash
git add services/frontend/
git commit -m "fix(m9): configure Tailwind CSS source paths for both frontend apps"
```

---

### Task 10: Add contracts list route to BFF Client

The client app's contracts list needs a BFF route that auto-injects `client_id`.

**Files:**
- Modify: `services/bff/client/cmd/server/contract_handler.go` (or new file if using pkg/bff)

**Step 1: Add List handler to client BFF**

Add `GET /api/v1/contracts` route to bff-client's contract handler that injects `?client_id={user_id}`:

```go
mux.HandleFunc("GET /api/v1/contracts", h.List)

func (h *ContractHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := bff.UserIDFrom(r.Context())
	if userID == "" {
		bff.WriteError(w, http.StatusUnauthorized, "missing user ID")
		return
	}
	path := "/api/v1/contracts?client_id=" + url.QueryEscape(userID)
	h.contracts.Forward(r.Context(), w, http.MethodGet, path, nil)
}
```

**Step 2: Test and commit**

Run: `cd services/bff/client && go test ./cmd/server/... -v`

```bash
git add services/bff/client/
git commit -m "feat(m9): add contracts list route to BFF Client

GET /api/v1/contracts auto-injects client_id from JWT context."
```

---

### Task 11: Final verification

**Step 1: Build and start all services**

```bash
make down && make up
make health
```

Expected: All services healthy.

**Step 2: Verify client app**

Open `http://localhost:5173`. Login as `client@example.com`. Verify:
- Top navigation renders (not sidebar)
- Job list shows cards (not table)
- Contracts list page works
- Wallet page works

**Step 3: Verify freelancer app**

Open `http://localhost:5174`. Login as `freelancer@example.com`. Verify:
- Top navigation renders with freelancer nav items
- Matches page loads (may show empty if no profile exists)
- Contracts page shows list
- Wallet page shows balance

**Step 4: Run all backend tests**

```bash
cd pkg/bff && go test ./... -v
cd services/bff/client && go test ./cmd/server/... -v
cd services/bff/freelancer && go test ./cmd/server/... -v
cd services/contracts && go test ./cmd/server/... -v
```

**Step 5: Run all frontend tests**

```bash
cd services/frontend
pnpm --filter @hire-flow/client test
pnpm --filter @hire-flow/freelancer test
```

**Step 6: Final commit (if any fixes needed)**

```bash
git commit -m "fix(m9): final adjustments from verification"
```

---

## Summary

| Phase | Task | What | Files |
|-------|------|------|-------|
| 1 | 1 | Extract pkg/bff/ shared package | ~12 files |
| 1 | 2 | Contracts list endpoint | ~4 files |
| 1 | 3 | BFF Freelancer implementation | ~8 files |
| 2 | 4 | Extract shared frontend code | ~8 files |
| 2 | 5 | Client app redesign | ~10 files |
| 3 | 6 | Scaffold freelancer app | ~10 files |
| 3 | 7 | Freelancer routes & features | ~18 files |
| 3 | 8 | Frontend tests | ~6 files |
| 3 | 9 | Tailwind source paths | ~2 files |
| 3 | 10 | Client BFF contracts list | ~1 file |
| 3 | 11 | Final verification | 0 files |

**Total: 11 tasks, ~80 files, estimated CC time: ~1.5 hours**
