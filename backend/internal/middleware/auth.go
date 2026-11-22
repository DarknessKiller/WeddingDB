package middleware

import (
	"context"
	"net/http"
	"strings"
	"weddingdb/internal/services"
)

type contextKey string

// ChangePasswordPath is the only authenticated route a token flagged for a
// forced password change may call; login, refresh and logout are public.
const ChangePasswordPath = "/api/auth/change-password"

const (
	AdminIDKey   contextKey = "adminId"
	WeddingIDKey contextKey = "weddingId"
	RoleKey      contextKey = "role"
	SessionIDKey contextKey = "sessionId"
)

func AuthMiddleware(authService *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearer(r)
			if token == "" {
				http.Error(w, `{"error":"Missing token"}`, http.StatusUnauthorized)
				return
			}

			// ValidateToken handles the blacklist and token-version checks.
			// Uses r.Context() for cancellation safety — if the client disconnects,
			// Redis calls respect context cancellation and the request is rejected.
			claims, err := authService.ValidateToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			if claims.ForcePasswordChange && r.URL.Path != ChangePasswordPath {
				http.Error(w, `{"error":"Password change required"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), AdminIDKey, claims.AdminID)
			ctx = context.WithValue(ctx, WeddingIDKey, claims.WeddingID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			ctx = context.WithValue(ctx, SessionIDKey, claims.SessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ExtractBearer returns the token from an HTTP Bearer authorization header.
func ExtractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
