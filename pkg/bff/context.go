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
