package bff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeToken(t *testing.T, sub, role string, exp time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  sub,
		"role": role,
		"exp":  exp.Unix(),
		"iat":  time.Now().Unix(),
		"type": "access",
	})
	s, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)
	return s
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	auth := newTestAuthConfig()

	var gotUserID, gotRole string
	handler := auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(CtxKeyUserID).(string)
		gotRole, _ = r.Context().Value(CtxKeyRole).(string)
		w.WriteHeader(http.StatusOK)
	}))

	token := makeToken(t, "user-1", "client", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-1", gotUserID)
	assert.Equal(t, "client", gotRole)
}

func TestJWTMiddleware_MissingCookie(t *testing.T) {
	auth := newTestAuthConfig()
	handler := auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	auth := newTestAuthConfig()
	handler := auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	token := makeToken(t, "user-1", "client", time.Now().Add(-time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_MalformedToken(t *testing.T) {
	auth := newTestAuthConfig()
	handler := auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "not-a-jwt"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_NoneAlgorithm(t *testing.T) {
	auth := newTestAuthConfig()
	handler := auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":  "attacker",
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	})
	s, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: s})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTMiddleware_RefreshTokenRejected(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-123",
		"role": "client",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
		"type": "refresh",
	})
	s, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)

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
	rl := NewRateLimiter(10, 10)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(1, 1)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-1"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	assert.Equal(t, "rate limit exceeded", body["error"])
}

func TestRateLimiter_IndependentBuckets(t *testing.T) {
	rl := NewRateLimiter(1, 1)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-1"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(WithUserID(req.Context(), "user-2"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
