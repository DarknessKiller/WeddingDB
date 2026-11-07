package handlers

import (
	"net/http"

	"weddingdb/internal/services"
)

// PublicRateMiddleware caps /api/public per client IP using the auth limiter.
func PublicRateMiddleware(as *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := as.CheckPublicRate(r.Context(), clientIP(r)); err != nil {
				http.Error(w, `{"error":"Too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
