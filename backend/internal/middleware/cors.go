package middleware

import (
	"net/http"
	"os"
)

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("CORS_ORIGIN")
		// Credentials are only allowed for the configured origin; reflecting
		// an arbitrary request Origin with credentials would let any website
		// read API responses.
		allowCredentials := origin != ""
		if origin == "" {
			origin = r.Header.Get("Origin")
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if allowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
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
