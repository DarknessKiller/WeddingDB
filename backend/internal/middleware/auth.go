package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"weddingdb/internal/services"
)

type contextKey string

// changePasswordPath is the one authenticated route a session flagged for a
// forced password change may call.
const changePasswordPath = "/api/auth/change-password"

const (
	AdminIDKey   contextKey = "adminId"
	WeddingIDKey contextKey = "weddingId"
	RoleKey      contextKey = "role"
)

func AuthMiddleware(authService *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearer(r)
			if token == "" {
				http.Error(w, `{"error":"Missing token"}`, http.StatusUnauthorized)
				return
			}

			// ValidateToken now handles blacklist + token_version checks via Redis.
			// Uses r.Context() for cancellation safety — if the client disconnects,
			// Redis calls respect context cancellation and the request is rejected.
			claims, err := authService.ValidateToken(r.Context(), token)
			if err != nil {
				WriteAuthError(w, err)
				return
			}

			// An admin-reset password must be replaced before anything else.
			// Logout and refresh are public routes, so only change-password stays open.
			if claims.ForcePasswordChange && r.URL.Path != changePasswordPath {
				http.Error(w, `{"error":"Password change required"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), AdminIDKey, claims.AdminID)
			ctx = context.WithValue(ctx, WeddingIDKey, claims.WeddingID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WriteAuthError answers a failed ValidateToken: 401 for a rejected token,
// 503 for an outage so the client keeps its session and retries.
func WriteAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, services.ErrTokenRejected) {
		http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
		return
	}
	slog.Error("token validation unavailable", "err", err)
	http.Error(w, `{"error":"service temporarily unavailable"}`, http.StatusServiceUnavailable)
}

// ExtractBearer returns the token from an HTTP Bearer authorization header.
func ExtractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
