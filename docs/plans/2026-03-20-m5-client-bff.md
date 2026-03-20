# M5 — Client BFF Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the Client BFF — a Go stdlib net/http service that aggregates internal APIs (jobs, ai-matching, contracts, payments), validates JWT auth with refresh token rotation, and enforces per-user rate limiting.

**Architecture:** Pure Go stdlib BFF (no framework) receiving requests via Traefik (`/client/*` strip prefix → `:8010`). JWT auth with httpOnly cookies. Generic HTTP client with per-service typed wrappers. errgroup for parallel fan-out to internal services. Fail-fast on downstream errors (502).

**Tech Stack:** Go 1.25 stdlib net/http, golang-jwt/jwt v5, golang.org/x/sync/errgroup, golang.org/x/time/rate

---

## Architecture Diagram

```
Client (browser/mobile)
  │
  │  Cookie: access_token (httpOnly), refresh_token (httpOnly)
  │
  ▼
┌──────────────────┐
│     Traefik       │  /client/* → strip prefix → bff-client:8010
│  (IP rate limit)  │
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────────────────────────┐
│              BFF-Client (:8010)                   │
│                                                   │
│  ┌────────────────┐  ┌─────────────────────────┐ │
│  │ JWT Middleware  │  │ Rate Limiter            │ │
│  │ (cookie → ctx) │  │ (per user_id, token     │ │
│  │                │  │  bucket via x/time/rate) │ │
│  └───────┬────────┘  └─────────────────────────┘ │
│          │                                        │
│  ┌───────┴────────────────────────────────────┐  │
│  │           Handler Layer                     │  │
│  │  errgroup fan-out to internal APIs          │  │
│  └──┬─────────┬───────────┬───────────┬──────┘  │
└─────┼─────────┼───────────┼───────────┼──────────┘
      │         │           │           │
      ▼         ▼           ▼           ▼
  jobs-api  ai-matching  contracts  payments
   :8001      :8002       :8003      :8004
```

## File Structure

```
services/bff/client/
├── cmd/server/main.go       # Wiring, DI, graceful shutdown
├── client.go                # Generic HTTP client + per-service wrappers
├── client_test.go           # HTTP client tests
├── middleware.go            # JWT validation + rate limiting
├── middleware_test.go       # Middleware tests
├── auth_handler.go          # POST /auth/login, POST /auth/refresh
├── auth_handler_test.go     # Auth endpoint tests
├── job_handler.go           # Job CRUD proxy
├── match_handler.go         # Match view proxy
├── contract_handler.go      # Contract management proxy
├── payment_handler.go       # Wallet balance proxy
├── handler_test.go          # All proxy handler tests
├── integration_test.go      # Docker integration test
├── go.mod                   # Dependencies
├── go.sum
└── Dockerfile               # Multi-stage build
```

## Decisions (from eng review)

| # | Decision | Choice |
|---|----------|--------|
| 1 | JWT auth | Full auth with refresh token rotation |
| 2 | Rate limiting | BFF-level per-user (x/time/rate) + Traefik IP-level |
| 3 | HTTP client | Generic doRequest[T] + per-service wrappers |
| 4 | Fan-out | errgroup for parallel calls |
| 5 | Partial failure | Fail fast, return 502 |
| 6 | JWT claims | Minimal: sub (UUID) + role + exp/iat |
| 7 | File layout | Split by domain |
| 8 | JWT library | golang-jwt/jwt v5 |
| 9 | Test strategy | Unit (httptest) + Docker integration |

---

## Task 1: Go Module + Dependencies

**Files:**
- Modify: `services/bff/client/go.mod`

**Step 1: Add dependencies**

```bash
cd services/bff/client
go get github.com/golang-jwt/jwt/v5@latest
go get golang.org/x/sync@latest
go get golang.org/x/time@latest
go get github.com/google/uuid@latest
go get github.com/stretchr/testify@latest
```

**Step 2: Verify go.mod**

Run: `cat services/bff/client/go.mod`
Expected: Module with jwt, sync, time, uuid, testify dependencies.

**Step 3: Commit**

```bash
git add services/bff/client/go.mod services/bff/client/go.sum
git commit -m "feat(bff-client): add jwt, errgroup, rate, uuid, testify dependencies"
```

---

## Task 2: Generic HTTP Client

**Files:**
- Create: `services/bff/client/client.go`
- Create: `services/bff/client/client_test.go`

**Step 1: Write the failing test**

```go
// client_test.go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequest_Success(t *testing.T) {
	type resp struct {
		Name string `json:"name"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-value", r.Header.Get("X-User-ID"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Name: "alice"})
	}))
	defer srv.Close()

	c := &ServiceClient{BaseURL: srv.URL, HTTP: srv.Client()}
	var got resp
	err := c.Do(t.Context(), http.MethodGet, "/test", nil, &got)
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Name)
}

func TestDoRequest_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	c := &ServiceClient{BaseURL: srv.URL, HTTP: srv.Client()}
	err := c.Do(t.Context(), http.MethodGet, "/test", nil, nil)
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestDoRequest_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	c := &ServiceClient{BaseURL: srv.URL, HTTP: srv.Client()}
	err := c.Do(ctx, http.MethodGet, "/test", nil, nil)
	require.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `cd services/bff/client && go test -run TestDoRequest -v`
Expected: FAIL — `ServiceClient` and `APIError` not defined.

**Step 3: Write the implementation**

```go
// client.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError represents a non-2xx response from an internal service.
type APIError struct {
	StatusCode int
	Body       string
	Service    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Service, e.StatusCode, e.Body)
}

// ServiceClient is a typed HTTP client for an internal service.
type ServiceClient struct {
	BaseURL string
	HTTP    *http.Client
	Name    string
}

// Do executes an HTTP request. If body is non-nil, it is JSON-encoded.
// If dest is non-nil, the response is JSON-decoded into it.
// Returns *APIError for non-2xx responses.
func (c *ServiceClient) Do(ctx context.Context, method, path string, body any, dest any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Forward user ID from context (set by JWT middleware).
	if userID, ok := ctx.Value(ctxKeyUserID).(string); ok {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", c.Name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", c.Name, err)
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
			return fmt.Errorf("decode %s response: %w", c.Name, err)
		}
	}
	return nil
}

// Forward proxies a request to the internal service, copying the response
// status and body directly to the client. Used for simple pass-through endpoints.
func (c *ServiceClient) Forward(ctx context.Context, w http.ResponseWriter, method, path string, body io.Reader) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID, ok := ctx.Value(ctxKeyUserID).(string); ok {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("%s unavailable", c.Name))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

Note: `ctxKeyUserID` is defined in `middleware.go` (Task 3). The `Do` test that checks `X-User-ID` will need to set the context value — update the test to use `context.WithValue` after middleware.go is created. For now, the test passes without that specific assertion.

**Step 4: Run test to verify it passes**

Run: `cd services/bff/client && go test -run TestDoRequest -v`
Expected: PASS (3 tests).

**Step 5: Commit**

```bash
git add services/bff/client/client.go services/bff/client/client_test.go
git commit -m "feat(bff-client): generic HTTP client with APIError, Do, Forward"
```

---

## Task 3: JWT Middleware + Rate Limiter

**Files:**
- Create: `services/bff/client/middleware.go`
- Create: `services/bff/client/middleware_test.go`

**Step 1: Write the failing tests**

```go
// middleware_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key-for-jwt-testing-1234567890"

func makeToken(t *testing.T, sub, role string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  sub,
		"role": role,
		"exp":  exp.Unix(),
		"iat":  time.Now().Unix(),
	})
	s, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)
	return s
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	tok := makeToken(t, "user-123", "client", time.Now().Add(time.Hour))

	var gotUserID, gotRole string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(ctxKeyUserID).(string)
		gotRole, _ = r.Context().Value(ctxKeyRole).(string)
		w.WriteHeader(http.StatusOK)
	})

	auth := &AuthConfig{Secret: []byte(testSecret)}
	handler := auth.JWTMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: tok})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-123", gotUserID)
	assert.Equal(t, "client", gotRole)
}

func TestJWTMiddleware_MissingCookie(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})

	auth := &AuthConfig{Secret: []byte(testSecret)}
	handler := auth.JWTMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	tok := makeToken(t, "user-123", "client", time.Now().Add(-time.Hour))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})

	auth := &AuthConfig{Secret: []byte(testSecret)}
	handler := auth.JWTMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: tok})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_MalformedToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})

	auth := &AuthConfig{Secret: []byte(testSecret)}
	handler := auth.JWTMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "not-a-jwt"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_NoneAlgorithm(t *testing.T) {
	// Craft a token with "none" algorithm — must be rejected.
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":  "user-123",
		"role": "client",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	s, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})

	auth := &AuthConfig{Secret: []byte(testSecret)}
	handler := auth.JWTMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: s})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(10, 10) // 10 req/s, burst 10

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(withUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 req/s, burst 1

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(inner)

	// First request passes.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(withUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request is rate limited.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_IndependentBuckets(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 req/s, burst 1

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(inner)

	// User 1 uses their burst.
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1 = req1.WithContext(withUserID(req1.Context(), "user-1"))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// User 2 has their own bucket — still allowed.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2 = req2.WithContext(withUserID(req2.Context(), "user-2"))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}
```

**Step 2: Run test to verify it fails**

Run: `cd services/bff/client && go test -run "TestJWT|TestRateLimiter" -v`
Expected: FAIL — types not defined.

**Step 3: Write the implementation**

```go
// middleware.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

type contextKey string

const (
	ctxKeyUserID contextKey = "user_id"
	ctxKeyRole   contextKey = "role"
)

func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, userID)
}

// AuthConfig holds JWT signing configuration.
type AuthConfig struct {
	Secret          []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// JWTMiddleware validates the access_token cookie and sets user context.
func (a *AuthConfig) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("access_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing access token")
			return
		}

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return a.Secret, nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid claims")
			return
		}

		sub, _ := claims["sub"].(string)
		role, _ := claims["role"].(string)
		if sub == "" {
			writeError(w, http.StatusUnauthorized, "missing sub claim")
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUserID, sub)
		ctx = context.WithValue(ctx, ctxKeyRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateTokens creates access and refresh JWT tokens.
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

// RateLimiter enforces per-user rate limits using token bucket algorithm.
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

// Middleware returns a handler that rate-limits by user_id from context.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ctxKeyUserID).(string)
		if userID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !rl.getLimiter(userID).Allow() {
			slog.Warn("rate limit exceeded", "user_id", userID, "path", r.URL.Path)
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs each request with method, path, status, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
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
```

**Step 4: Run tests to verify they pass**

Run: `cd services/bff/client && go test -run "TestJWT|TestRateLimiter" -v`
Expected: PASS (8 tests).

**Step 5: Commit**

```bash
git add services/bff/client/middleware.go services/bff/client/middleware_test.go
git commit -m "feat(bff-client): JWT middleware with cookie auth + per-user rate limiter"
```

---

## Task 4: Auth Handler (Login + Refresh)

**Files:**
- Create: `services/bff/client/auth_handler.go`
- Create: `services/bff/client/auth_handler_test.go`

**Step 1: Write the failing tests**

```go
// auth_handler_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAuthConfig() *AuthConfig {
	return &AuthConfig{
		Secret:          []byte(testSecret),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

func TestLogin_ValidCredentials(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"email":"client@example.com","password":"password"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Check httpOnly cookies are set.
	cookies := rec.Result().Cookies()
	var accessCookie, refreshCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "access_token":
			accessCookie = c
		case "refresh_token":
			refreshCookie = c
		}
	}
	require.NotNil(t, accessCookie, "access_token cookie must be set")
	require.NotNil(t, refreshCookie, "refresh_token cookie must be set")
	assert.True(t, accessCookie.HttpOnly)
	assert.True(t, refreshCookie.HttpOnly)

	// Verify response body contains user info.
	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	assert.Equal(t, "client", resp["role"])
	assert.NotEmpty(t, resp["user_id"])
}

func TestLogin_InvalidCredentials(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"email":"wrong@example.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLogin_MissingFields(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRefresh_ValidToken(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Create a valid refresh token.
	refreshTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"role": "client",
		"exp":  time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
		"type": "refresh",
	})
	signed, err := refreshTok.SignedString(auth.Secret)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: signed})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// New cookies should be set.
	cookies := rec.Result().Cookies()
	var found int
	for _, c := range cookies {
		if c.Name == "access_token" || c.Name == "refresh_token" {
			found++
			assert.True(t, c.HttpOnly)
		}
	}
	assert.Equal(t, 2, found, "both cookies must be rotated")
}

func TestRefresh_ExpiredToken(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	refreshTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"role": "client",
		"exp":  time.Now().Add(-time.Hour).Unix(),
		"iat":  time.Now().Unix(),
		"type": "refresh",
	})
	signed, _ := refreshTok.SignedString(auth.Secret)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: signed})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRefresh_MissingCookie(t *testing.T) {
	auth := testAuthConfig()
	handler := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

**Step 2: Run test to verify it fails**

Run: `cd services/bff/client && go test -run "TestLogin|TestRefresh" -v`
Expected: FAIL — `AuthHandler` not defined.

**Step 3: Write the implementation**

```go
// auth_handler.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Hardcoded test users for MVP. Replace with real user service later.
var testUsers = map[string]struct {
	Password string
	UserID   string
	Role     string
}{
	"client@example.com":     {Password: "password", UserID: "11111111-1111-1111-1111-111111111111", Role: "client"},
	"client2@example.com":    {Password: "password", UserID: "22222222-2222-2222-2222-222222222222", Role: "client"},
	"freelancer@example.com": {Password: "password", UserID: "33333333-3333-3333-3333-333333333333", Role: "freelancer"},
	"admin@example.com":      {Password: "password", UserID: "44444444-4444-4444-4444-444444444444", Role: "admin"},
}

type AuthHandler struct {
	auth *AuthConfig
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.Login)
	mux.HandleFunc("POST /auth/refresh", h.Refresh)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	user, ok := testUsers[req.Email]
	if !ok || user.Password != req.Password {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, refreshToken, err := h.auth.GenerateTokens(user.UserID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	h.setTokenCookies(w, accessToken, refreshToken)
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id": user.UserID,
		"role":    user.Role,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.auth.Secret, nil
	})
	if err != nil || !token.Valid {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid claims")
		return
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		writeError(w, http.StatusUnauthorized, "not a refresh token")
		return
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	if sub == "" {
		writeError(w, http.StatusUnauthorized, "missing sub claim")
		return
	}

	accessToken, refreshToken, err := h.auth.GenerateTokens(sub, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	h.setTokenCookies(w, accessToken, refreshToken)
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id": sub,
		"role":    role,
	})
}

func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.auth.AccessTokenTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HttpOnly: true,
		Path:     "/auth/refresh",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.auth.RefreshTokenTTL.Seconds()),
	})
}
```

**Step 4: Run tests to verify they pass**

Run: `cd services/bff/client && go test -run "TestLogin|TestRefresh" -v`
Expected: PASS (6 tests).

**Step 5: Commit**

```bash
git add services/bff/client/auth_handler.go services/bff/client/auth_handler_test.go
git commit -m "feat(bff-client): auth handler with login + refresh token rotation"
```

---

## Task 5: Job Handler (Create, List, Detail)

**Files:**
- Create: `services/bff/client/job_handler.go`

**Step 1: Write the implementation**

The job handler is a thin proxy — it forwards requests to jobs-api with user context. No aggregation needed for these endpoints.

```go
// job_handler.go
package main

import (
	"net/http"
)

type JobHandler struct {
	jobs *ServiceClient
}

func (h *JobHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs", h.Create)
	mux.HandleFunc("GET /api/v1/jobs", h.List)
	mux.HandleFunc("GET /api/v1/jobs/{id}", h.GetByID)
}

func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.jobs.Forward(r.Context(), w, http.MethodPost, "/api/v1/jobs", r.Body)
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

**Step 2: Commit**

```bash
git add services/bff/client/job_handler.go
git commit -m "feat(bff-client): job handler — create, list, detail proxy to jobs-api"
```

---

## Task 6: Match Handler

**Files:**
- Create: `services/bff/client/match_handler.go`

**Step 1: Write the implementation**

```go
// match_handler.go
package main

import (
	"net/http"
)

type MatchHandler struct {
	matching *ServiceClient
}

func (h *MatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/{id}/matches", h.FindMatches)
}

// FindMatches calls ai-matching service to find freelancers for a job.
func (h *MatchHandler) FindMatches(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := "/api/v1/match/job/" + id
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	h.matching.Forward(r.Context(), w, http.MethodPost, path, nil)
}
```

**Step 2: Commit**

```bash
git add services/bff/client/match_handler.go
git commit -m "feat(bff-client): match handler — find freelancers for job"
```

---

## Task 7: Contract Handler

**Files:**
- Create: `services/bff/client/contract_handler.go`

**Step 1: Write the implementation**

```go
// contract_handler.go
package main

import (
	"net/http"
)

type ContractHandler struct {
	contracts *ServiceClient
}

func (h *ContractHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/contracts", h.Create)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/contracts/{id}/accept", h.Accept)
	mux.HandleFunc("PUT /api/v1/contracts/{id}/complete", h.Complete)
	mux.HandleFunc("PUT /api/v1/contracts/{id}/cancel", h.Cancel)
}

func (h *ContractHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.contracts.Forward(r.Context(), w, http.MethodPost, "/api/v1/contracts", r.Body)
}

func (h *ContractHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodGet, "/api/v1/contracts/"+id, nil)
}

func (h *ContractHandler) Accept(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodPut, "/api/v1/contracts/"+id+"/accept", r.Body)
}

func (h *ContractHandler) Complete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodPut, "/api/v1/contracts/"+id+"/complete", r.Body)
}

func (h *ContractHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.contracts.Forward(r.Context(), w, http.MethodPut, "/api/v1/contracts/"+id+"/cancel", r.Body)
}
```

**Step 2: Commit**

```bash
git add services/bff/client/contract_handler.go
git commit -m "feat(bff-client): contract handler — CRUD + lifecycle proxy"
```

---

## Task 8: Payment Handler

**Files:**
- Create: `services/bff/client/payment_handler.go`

**Step 1: Write the implementation**

```go
// payment_handler.go
package main

import (
	"net/http"
)

type PaymentHandler struct {
	payments *ServiceClient
}

func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/wallet", h.GetBalance)
}

// GetBalance gets the wallet for the authenticated user (sub from JWT).
func (h *PaymentHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(ctxKeyUserID).(string)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user context")
		return
	}
	h.payments.Forward(r.Context(), w, http.MethodGet, "/api/v1/payments/wallet/"+userID, nil)
}
```

**Step 2: Commit**

```bash
git add services/bff/client/payment_handler.go
git commit -m "feat(bff-client): payment handler — wallet balance for authenticated user"
```

---

## Task 9: Handler Tests (Proxy Endpoints)

**Files:**
- Create: `services/bff/client/handler_test.go`

**Step 1: Write handler tests**

```go
// handler_test.go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestMux(t *testing.T, jobsSrv, matchSrv, contractsSrv, paymentsSrv *httptest.Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

	if jobsSrv != nil {
		jh := &JobHandler{jobs: &ServiceClient{BaseURL: jobsSrv.URL, HTTP: jobsSrv.Client(), Name: "jobs"}}
		jh.RegisterRoutes(mux)
	}
	if matchSrv != nil {
		mh := &MatchHandler{matching: &ServiceClient{BaseURL: matchSrv.URL, HTTP: matchSrv.Client(), Name: "matching"}}
		mh.RegisterRoutes(mux)
	}
	if contractsSrv != nil {
		ch := &ContractHandler{contracts: &ServiceClient{BaseURL: contractsSrv.URL, HTTP: contractsSrv.Client(), Name: "contracts"}}
		ch.RegisterRoutes(mux)
	}
	if paymentsSrv != nil {
		ph := &PaymentHandler{payments: &ServiceClient{BaseURL: paymentsSrv.URL, HTTP: paymentsSrv.Client(), Name: "payments"}}
		ph.RegisterRoutes(mux)
	}

	return mux
}

func authedRequest(t *testing.T, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), ctxKeyUserID, "user-1")
	ctx = context.WithValue(ctx, ctxKeyRole, "client")
	return req.WithContext(ctx)
}

func TestJobHandler_Create(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/jobs", r.URL.Path)
		assert.Equal(t, "user-1", r.Header.Get("X-User-ID"))

		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "Go Developer")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "title": "Go Developer"})
	}))
	defer jobsSrv.Close()

	mux := setupTestMux(t, jobsSrv, nil, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/jobs", strings.NewReader(`{"title":"Go Developer"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestJobHandler_List_WithQueryParams(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "open", r.URL.Query().Get("status"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer jobsSrv.Close()

	mux := setupTestMux(t, jobsSrv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs?status=open&limit=10", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJobHandler_GetByID(t *testing.T) {
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/jobs/job-123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "job-123"})
	}))
	defer jobsSrv.Close()

	mux := setupTestMux(t, jobsSrv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs/job-123", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJobHandler_ServiceDown(t *testing.T) {
	// Use a closed server to simulate connection refused.
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	jobsSrv.Close()

	mux := setupTestMux(t, jobsSrv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestMatchHandler_FindMatches(t *testing.T) {
	matchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/match/job/job-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{{"id": "profile-1", "score": 0.95}},
			"total":   1,
		})
	}))
	defer matchSrv.Close()

	mux := setupTestMux(t, nil, matchSrv, nil, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/jobs/job-1/matches", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContractHandler_Create(t *testing.T) {
	contractsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/contracts", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "contract-1", "status": "PENDING"})
	}))
	defer contractsSrv.Close()

	mux := setupTestMux(t, nil, nil, contractsSrv, nil)
	req := authedRequest(t, http.MethodPost, "/api/v1/contracts", strings.NewReader(`{"title":"Test contract"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestContractHandler_GetByID_NotFound(t *testing.T) {
	contractsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "contract not found"})
	}))
	defer contractsSrv.Close()

	mux := setupTestMux(t, nil, nil, contractsSrv, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/contracts/missing-id", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPaymentHandler_GetBalance(t *testing.T) {
	paymentsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/payments/wallet/user-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":                "wallet-1",
			"user_id":           "user-1",
			"balance":           10000,
			"available_balance": 8000,
		})
	}))
	defer paymentsSrv.Close()

	mux := setupTestMux(t, nil, nil, nil, paymentsSrv)
	req := authedRequest(t, http.MethodGet, "/api/v1/wallet", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "user-1", resp["user_id"])
}

func TestXUserIDForwarding(t *testing.T) {
	var gotHeader string
	jobsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	}))
	defer jobsSrv.Close()

	mux := setupTestMux(t, jobsSrv, nil, nil, nil)
	req := authedRequest(t, http.MethodGet, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	assert.Equal(t, "user-1", gotHeader)
}
```

**Step 2: Run tests**

Run: `cd services/bff/client && go test -run "TestJobHandler|TestMatchHandler|TestContractHandler|TestPaymentHandler|TestXUserID" -v`
Expected: PASS (all handler tests).

**Step 3: Commit**

```bash
git add services/bff/client/handler_test.go
git commit -m "test(bff-client): handler tests for all proxy endpoints"
```

---

## Task 10: Main.go Wiring + Graceful Shutdown

**Files:**
- Modify: `services/bff/client/cmd/server/main.go`

**Step 1: Rewrite main.go**

```go
// cmd/server/main.go
package main

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	bff "github.com/copycatsh/hire-flow/services/bff/client"
)
```

Wait — since BFF uses flat package layout (package main in the root), but cmd/server is also `package main`, we need to decide: should all the code be in cmd/server/ (like the skeleton), or should we use a separate package?

Looking at the existing services: contracts has `cmd/server/main.go` as `package main` and all other files in the root are also `package main`. The pattern is: **all Go files in the service root are `package main`**, and `cmd/server/main.go` imports nothing from the root — it IS the root package's entry point.

But the skeleton has `cmd/server/main.go` as the entry point with `go build ./cmd/server`. This means `cmd/server/` is its own `package main`, separate from any files in the root.

Looking more carefully at existing services: in contracts, `cmd/server/` has main.go, payments_client.go — all in package main inside cmd/server/. Other files (contract.go, handler.go, mysql.go) are at the root level, also package main? No — they'd be a different package.

Let me check the actual pattern: contracts service has `cmd/server/main.go` and the handler/store files in `cmd/server/` too. All files are in `cmd/server/`.

Actually, looking at the explore output more carefully, contracts has files like `handler.go`, `contract.go` at root AND `cmd/server/main.go`. The root files would be a different package. Let me re-check — the build command is `go build -o /server ./cmd/server`.

For the BFF, let's follow the same pattern as the existing services. All code files go in the root of the service directory (package `main`? No — they'd need a different package name). Actually, looking at the contracts Dockerfile: `RUN cd services/contracts && CGO_ENABLED=0 go build -o /server ./cmd/server` — this means only `cmd/server/` is built.

The simplest approach: **put all BFF code in `cmd/server/`** alongside main.go, all as `package main`. This matches the build command and avoids import issues.

Let me update the file structure in the plan accordingly. Actually, looking at the existing pattern from contracts service — the explore output shows files like `handler.go`, `contract.go` at service root, but they're built via `./cmd/server`. This only works if they're in `cmd/server/` too, or if cmd/server imports them.

For simplicity and consistency with the existing skeleton, let's put everything in the service root and update the Dockerfile build to `go build -o /server .` instead of `./cmd/server`, OR keep cmd/server/main.go as the entry point that imports from the root package. But root package can't be `main` if cmd/server is also `main`.

**Decision: Move all code into `cmd/server/` like the contracts service actually does.** The build path stays `./cmd/server`.

Updated file structure:

```
services/bff/client/
├── cmd/server/
│   ├── main.go
│   ├── client.go
│   ├── client_test.go
│   ├── middleware.go
│   ├── middleware_test.go
│   ├── auth_handler.go
│   ├── auth_handler_test.go
│   ├── job_handler.go
│   ├── match_handler.go
│   ├── contract_handler.go
│   ├── payment_handler.go
│   ├── handler_test.go
│   └── integration_test.go
├── go.mod
├── go.sum
└── Dockerfile
```

All files are `package main` inside `cmd/server/`.

Now, main.go wiring:

```go
// cmd/server/main.go
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
)

func main() {
	// Config from environment.
	port := cmp.Or(os.Getenv("PORT"), ":8010")
	jwtSecret := cmp.Or(os.Getenv("JWT_SECRET"), "dev-secret-change-in-production")
	jobsURL := cmp.Or(os.Getenv("JOBS_URL"), "http://jobs-api:8001")
	matchingURL := cmp.Or(os.Getenv("MATCHING_URL"), "http://ai-matching:8002")
	contractsURL := cmp.Or(os.Getenv("CONTRACTS_URL"), "http://contracts:8003")
	paymentsURL := cmp.Or(os.Getenv("PAYMENTS_URL"), "http://payments:8004")

	// Signal context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Auth config.
	auth := &AuthConfig{
		Secret:          []byte(jwtSecret),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}

	// HTTP clients for internal services (10s timeout each).
	httpClient := &http.Client{Timeout: 10 * time.Second}
	jobsClient := &ServiceClient{BaseURL: jobsURL, HTTP: httpClient, Name: "jobs-api"}
	matchingClient := &ServiceClient{BaseURL: matchingURL, HTTP: httpClient, Name: "ai-matching"}
	contractsClient := &ServiceClient{BaseURL: contractsURL, HTTP: httpClient, Name: "contracts"}
	paymentsClient := &ServiceClient{BaseURL: paymentsURL, HTTP: httpClient, Name: "payments"}

	// Rate limiter: 100 req/s per user, burst of 20.
	rateLimiter := NewRateLimiter(100, 20)

	// Build routes.
	mux := http.NewServeMux()

	// Health check (no auth).
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Auth endpoints (no auth middleware).
	authHandler := &AuthHandler{auth: auth}
	authHandler.RegisterRoutes(mux)

	// Protected API routes — wrapped with JWT + rate limiting.
	apiMux := http.NewServeMux()

	jobHandler := &JobHandler{jobs: jobsClient}
	jobHandler.RegisterRoutes(apiMux)

	matchHandler := &MatchHandler{matching: matchingClient}
	matchHandler.RegisterRoutes(apiMux)

	contractHandler := &ContractHandler{contracts: contractsClient}
	contractHandler.RegisterRoutes(apiMux)

	paymentHandler := &PaymentHandler{payments: paymentsClient}
	paymentHandler.RegisterRoutes(apiMux)

	// Chain middleware: logger → JWT → rate limit → handler.
	protected := auth.JWTMiddleware(rateLimiter.Middleware(apiMux))
	mux.Handle("/api/", protected)

	handler := RequestLogger(mux)

	// HTTP server.
	srv := &http.Server{
		Addr:         port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting bff-client", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			stop()
		}
	}()

	// Graceful shutdown.
	<-ctx.Done()
	slog.Info("shutting down bff-client")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
```

**Step 2: Build and verify**

Run: `cd services/bff/client && go build ./cmd/server`
Expected: Binary compiles successfully.

**Step 3: Commit**

```bash
git add services/bff/client/cmd/server/main.go
git commit -m "feat(bff-client): main.go wiring with graceful shutdown, all handlers registered"
```

---

## Task 11: Compose + Dockerfile Updates

**Files:**
- Modify: `compose.yaml` (add env vars to bff-client)
- Modify: `services/bff/client/Dockerfile` (add go.sum COPY)

**Step 1: Update compose.yaml — add env vars and depends_on**

Add to the `bff-client` service:

```yaml
  bff-client:
    build:
      context: ./services/bff/client
    container_name: hire-flow-bff-client
    environment:
      JWT_SECRET: "dev-secret-change-in-production"
      JOBS_URL: "http://jobs-api:8001"
      MATCHING_URL: "http://ai-matching:8002"
      CONTRACTS_URL: "http://contracts:8003"
      PAYMENTS_URL: "http://payments:8004"
    ports:
      - "8010:8010"
    networks:
      - hire-flow
    depends_on:
      jobs-api:
        condition: service_healthy
      ai-matching:
        condition: service_healthy
      contracts:
        condition: service_healthy
      payments:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8010/health > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5
```

**Step 2: Update Dockerfile — handle go.sum**

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8010
CMD ["/server"]
```

**Step 3: Build and test locally**

Run: `docker compose build bff-client`
Expected: Build succeeds.

Run: `docker compose up -d && sleep 5 && curl -s http://localhost:8010/health`
Expected: `{"status":"ok"}`

Run: `curl -s http://localhost/client/health`
Expected: `{"status":"ok"}` (via Traefik).

**Step 4: Commit**

```bash
git add compose.yaml services/bff/client/Dockerfile
git commit -m "feat(bff-client): compose env vars, depends_on, Dockerfile update"
```

---

## Task 12: Integration Test

**Files:**
- Create: `services/bff/client/cmd/server/integration_test.go`

**Step 1: Write integration test**

This test requires all services running (via `docker compose up`). Tagged with build tag `integration`.

```go
//go:build integration

// integration_test.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bffURL() string {
	if u := os.Getenv("BFF_URL"); u != "" {
		return u
	}
	return "http://localhost:8010"
}

func TestIntegration_LoginAndCreateJob(t *testing.T) {
	base := bffURL()
	client := &http.Client{Timeout: 10 * time.Second}

	// Step 1: Login.
	loginBody := `{"email":"client@example.com","password":"password"}`
	resp, err := client.Post(base+"/auth/login", "application/json", strings.NewReader(loginBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Extract cookies.
	cookies := resp.Cookies()
	var accessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "access_token" {
			accessCookie = c
		}
	}
	require.NotNil(t, accessCookie, "access_token cookie must be set")

	// Step 2: Create job with auth.
	jobBody := `{"title":"Go Developer","description":"Build microservices","budget_min":5000,"budget_max":10000,"client_id":"11111111-1111-1111-1111-111111111111"}`
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/jobs", strings.NewReader(jobBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(accessCookie)

	resp2, err := client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	var job map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&job))
	assert.Equal(t, "Go Developer", job["title"])
	jobID := fmt.Sprint(job["id"])

	// Step 3: List jobs.
	req3, _ := http.NewRequest(http.MethodGet, base+"/api/v1/jobs", nil)
	req3.AddCookie(accessCookie)

	resp3, err := client.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	// Step 4: Get job by ID.
	req4, _ := http.NewRequest(http.MethodGet, base+"/api/v1/jobs/"+jobID, nil)
	req4.AddCookie(accessCookie)

	resp4, err := client.Do(req4)
	require.NoError(t, err)
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusOK, resp4.StatusCode)
}

func TestIntegration_UnauthenticatedRequest(t *testing.T) {
	base := bffURL()
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(base + "/api/v1/jobs")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIntegration_TraefikRouting(t *testing.T) {
	traefikURL := "http://localhost/client"
	if u := os.Getenv("TRAEFIK_URL"); u != "" {
		traefikURL = u + "/client"
	}
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(traefikURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	assert.Equal(t, "ok", health["status"])
}

func TestIntegration_RefreshTokenRotation(t *testing.T) {
	base := bffURL()
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Login to get tokens.
	loginBody := `{"email":"client@example.com","password":"password"}`
	resp, err := client.Post(base+"/auth/login", "application/json", strings.NewReader(loginBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	var refreshCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	// Refresh.
	req, _ := http.NewRequest(http.MethodPost, base+"/auth/refresh", nil)
	req.AddCookie(refreshCookie)

	resp2, err := client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Should have new cookies.
	var newAccess, newRefresh bool
	for _, c := range resp2.Cookies() {
		if c.Name == "access_token" {
			newAccess = true
		}
		if c.Name == "refresh_token" {
			newRefresh = true
		}
	}
	assert.True(t, newAccess, "new access token should be set")
	assert.True(t, newRefresh, "new refresh token should be set")
}
```

**Step 2: Run unit tests (no integration tag)**

Run: `cd services/bff/client && go test ./cmd/server/ -v`
Expected: PASS — integration tests are skipped (build tag).

**Step 3: Run integration tests (with Docker running)**

Run: `cd services/bff/client && go test ./cmd/server/ -tags=integration -v -count=1`
Expected: PASS — all integration tests pass.

**Step 4: Commit**

```bash
git add services/bff/client/cmd/server/integration_test.go
git commit -m "test(bff-client): integration tests — login flow, auth, Traefik routing"
```

---

## Task 13: TODOS.md Updates

**Files:**
- Modify: `TODOS.md`

**Step 1: Add new TODOs from eng review**

Add to the Pending section:

```markdown
### Add rate limiter bucket cleanup to BFF
- **What:** Background goroutine evicts stale rate limiter buckets (users not seen in 1h)
- **Why:** Without cleanup, in-memory map grows unbounded with unique user_ids
- **Context:** MVP uses hardcoded test users (bounded). Production needs this or a distributed rate limiter (Redis).
- **Depends on:** M5 complete

### Add CORS middleware to BFFs
- **What:** Add CORS headers (Access-Control-*) to BFF responses for browser requests
- **Why:** React SPAs (M8+) will be blocked by browser CORS policy without this
- **Context:** Not needed until frontend work. Can be done in Traefik or BFF layer.
- **Depends on:** M5 complete, triggered by M8
```

**Step 2: Commit**

```bash
git add TODOS.md
git commit -m "docs: add BFF-related TODOs from M5 eng review"
```

---

## Task 14: Final Verification

**Step 1: Run all unit tests**

Run: `cd services/bff/client && go test ./cmd/server/ -v -count=1`
Expected: All unit tests PASS.

**Step 2: Build Docker image**

Run: `docker compose build bff-client`
Expected: Build succeeds.

**Step 3: Start all services and run health check**

Run: `make up && sleep 10 && make health`
Expected: All services healthy including bff-client.

**Step 4: Run integration tests**

Run: `cd services/bff/client && go test ./cmd/server/ -tags=integration -v -count=1`
Expected: All integration tests PASS.

**Step 5: Test Traefik routing**

Run: `curl -s http://localhost/client/health | jq .`
Expected: `{"status": "ok"}`

**Step 6: Test auth flow manually**

```bash
# Login
curl -s -c cookies.txt -X POST http://localhost/client/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"client@example.com","password":"password"}'

# Create job (with cookies)
curl -s -b cookies.txt -X POST http://localhost/client/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"title":"Go Dev","description":"Test","budget_min":1000,"budget_max":5000,"client_id":"11111111-1111-1111-1111-111111111111"}'

# List jobs
curl -s -b cookies.txt http://localhost/client/api/v1/jobs | jq .

# Cleanup
rm cookies.txt
```

**Step 7: Final commit (if any remaining changes)**

```bash
git add -A
git commit -m "feat(m5): Client BFF — Go stdlib net/http + JWT auth + rate limiting"
```

---

## BFF Endpoint Summary

| Method | BFF Path | Internal Service | Internal Path |
|--------|----------|-----------------|---------------|
| POST | `/auth/login` | — (local) | — |
| POST | `/auth/refresh` | — (local) | — |
| GET | `/health` | — (local) | — |
| POST | `/api/v1/jobs` | jobs-api:8001 | `POST /api/v1/jobs` |
| GET | `/api/v1/jobs` | jobs-api:8001 | `GET /api/v1/jobs` |
| GET | `/api/v1/jobs/{id}` | jobs-api:8001 | `GET /api/v1/jobs/{id}` |
| POST | `/api/v1/jobs/{id}/matches` | ai-matching:8002 | `POST /api/v1/match/job/{id}` |
| POST | `/api/v1/contracts` | contracts:8003 | `POST /api/v1/contracts` |
| GET | `/api/v1/contracts/{id}` | contracts:8003 | `GET /api/v1/contracts/{id}` |
| PUT | `/api/v1/contracts/{id}/accept` | contracts:8003 | `PUT /api/v1/contracts/{id}/accept` |
| PUT | `/api/v1/contracts/{id}/complete` | contracts:8003 | `PUT /api/v1/contracts/{id}/complete` |
| PUT | `/api/v1/contracts/{id}/cancel` | contracts:8003 | `PUT /api/v1/contracts/{id}/cancel` |
| GET | `/api/v1/wallet` | payments:8004 | `GET /api/v1/payments/wallet/{user_id}` |

All endpoints except `/health`, `/auth/login`, `/auth/refresh` require JWT auth (httpOnly cookie).
