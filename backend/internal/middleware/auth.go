package middleware

import (
	"context"
	"net/http"
	"strings"
	"weddingdb/internal/services"
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
			claims, err := authService.ValidateToken(token)
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

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
