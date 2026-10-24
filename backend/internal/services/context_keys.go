package services

import "context"

// Request-scoped context keys set by the auth middleware. Defined here so
// service code can read them without importing the middleware package
// (which itself imports services).
type contextKey string

const (
	AdminIDKey   contextKey = "adminId"
	WeddingIDKey contextKey = "weddingId"
	RoleKey      contextKey = "role"
)

// AdminIDFromCtx returns the authenticated admin's ID, or nil if absent.
func AdminIDFromCtx(ctx context.Context) (any, bool) {
	v := ctx.Value(AdminIDKey)
	return v, v != nil
}
