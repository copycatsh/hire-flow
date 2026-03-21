package bff

import (
	"context"
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
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Auth.CookieSecure,
		SameSite: sameSite,
		MaxAge:   int(h.Auth.RefreshTokenTTL.Seconds()),
	})
}
