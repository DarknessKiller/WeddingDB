package middleware

import (
	"net/http"
	"os"
)

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow only the configured origin. An unset CORS_ORIGIN means the API
		// is same-origin (the dev proxy or the bundled frontend), so no
		// cross-origin site is allowed — reflecting the request Origin here
		// would let any website read authenticated responses.
		allowed := os.Getenv("CORS_ORIGIN")
		w.Header().Add("Vary", "Origin")
		if allowed != "" && r.Header.Get("Origin") == allowed {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			// Credentials are only sent to this exact origin.
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		SetSecurityHeaders(w)
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
