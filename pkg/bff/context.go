package bff

import (
	"context"
	"net/http"
)

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
