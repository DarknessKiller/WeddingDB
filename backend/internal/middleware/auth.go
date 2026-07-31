package middleware

import (
	"context"
	"net/http"
	"strings"
	"weddingdb/internal/services"

	"github.com/google/uuid"
)

type contextKey string

const (
	AdminIDKey   contextKey = "adminId"
	WeddingIDKey contextKey = "weddingId"
	RoleKey      contextKey = "role"
)

func AuthMiddleware(authService *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				http.Error(w, `{"error":"Missing token"}`, http.StatusUnauthorized)
				return
			}

			// ValidateToken now handles blacklist + token_version checks via Redis.
			// Uses r.Context() for cancellation safety — if the client disconnects,
			// Redis calls respect context cancellation and the request is rejected.
			claims, err := authService.ValidateToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), AdminIDKey, claims.AdminID)
			ctx = context.WithValue(ctx, WeddingIDKey, claims.WeddingID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ParseUUID is a convenience wrapper around uuid.Parse that returns uuid.Nil on error.
func ParseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
