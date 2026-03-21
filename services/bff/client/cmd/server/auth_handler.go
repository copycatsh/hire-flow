package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

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

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, ok := testUsers[req.Email]
	if !ok || user.Password != req.Password {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, refreshToken, err := h.auth.GenerateTokens(user.UserID, user.Role)
	if err != nil {
		slog.Error("failed to generate tokens", "user_id", user.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate tokens")
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
		writeError(w, http.StatusUnauthorized, "invalid token type")
		return
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	if sub == "" || role == "" {
		writeError(w, http.StatusUnauthorized, "missing required claims")
		return
	}

	accessToken, refreshToken, err := h.auth.GenerateTokens(sub, role)
	if err != nil {
		slog.Error("failed to generate tokens", "user_id", sub, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	h.setTokenCookies(w, accessToken, refreshToken)
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id": sub,
		"role":    role,
	})
}

func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	sameSite := http.SameSiteStrictMode
	if !h.auth.CookieSecure {
		sameSite = http.SameSiteLaxMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.auth.CookieSecure,
		SameSite: sameSite,
		MaxAge:   int(h.auth.AccessTokenTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/auth/refresh",
		HttpOnly: true,
		Secure:   h.auth.CookieSecure,
		SameSite: sameSite,
		MaxAge:   int(h.auth.RefreshTokenTTL.Seconds()),
	})
}