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

func newAuthHandler(t *testing.T) (*AuthHandler, *http.ServeMux) {
	t.Helper()
	auth := newTestAuthConfig()
	h := &AuthHandler{auth: auth}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestLogin_ValidCredentials(t *testing.T) {
	_, mux := newAuthHandler(t)

	body := `{"email":"client@example.com","password":"password"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resp["user_id"])
	assert.Equal(t, "client", resp["role"])

	cookies := w.Result().Cookies()
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
	assert.True(t, accessCookie.HttpOnly)
	assert.Equal(t, "/", accessCookie.Path)

	require.NotNil(t, refreshCookie, "refresh_token cookie must be set")
	assert.True(t, refreshCookie.HttpOnly)
	assert.Equal(t, "/auth/refresh", refreshCookie.Path)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	_, mux := newAuthHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{"wrong password", `{"email":"client@example.com","password":"wrong"}`},
		{"unknown email", `{"email":"unknown@example.com","password":"password"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestLogin_MissingFields(t *testing.T) {
	_, mux := newAuthHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{"missing email", `{"password":"password"}`},
		{"missing password", `{"email":"client@example.com"}`},
		{"both empty", `{"email":"","password":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestRefresh_ValidToken(t *testing.T) {
	h, mux := newAuthHandler(t)

	_, refreshToken, err := h.auth.GenerateTokens("user-1", "client")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "user-1", resp["user_id"])
	assert.Equal(t, "client", resp["role"])

	cookies := w.Result().Cookies()
	var hasAccess, hasRefresh bool
	for _, c := range cookies {
		switch c.Name {
		case "access_token":
			hasAccess = true
		case "refresh_token":
			hasRefresh = true
		}
	}
	assert.True(t, hasAccess, "rotated access_token cookie must be set")
	assert.True(t, hasRefresh, "rotated refresh_token cookie must be set")
}

func TestRefresh_ExpiredToken(t *testing.T) {
	h, mux := newAuthHandler(t)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"role": "client",
		"exp":  time.Now().Add(-1 * time.Hour).Unix(),
		"iat":  time.Now().Add(-2 * time.Hour).Unix(),
		"type": "refresh",
	})
	expired, err := token.SignedString(h.auth.Secret)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: expired})
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefresh_MissingCookie(t *testing.T) {
	_, mux := newAuthHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}