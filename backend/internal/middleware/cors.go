package middleware

import (
	"net/http"
	"net/url"
	"os"
	"slices"
)

// localDevHosts are loopback names allowed to call the API from another port
// while CORS_ORIGIN is unset. Any other unconfigured origin gets no
// Allow-Origin, so a random website cannot read cross-origin responses.
var localDevHosts = []string{"localhost", "127.0.0.1", "::1"}

func isLocalDevOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return slices.Contains(localDevHosts, u.Hostname())
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configured := os.Getenv("CORS_ORIGIN")
		requestOrigin := r.Header.Get("Origin")
		// The allowed origin depends on the request, so caches must key on it.
		w.Header().Add("Vary", "Origin")
		switch {
		case configured != "":
			// Credentials are only allowed for the configured origin; reflecting
			// an arbitrary request Origin with credentials would let any website
			// read API responses.
			w.Header().Set("Access-Control-Allow-Origin", configured)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		case isLocalDevOrigin(requestOrigin):
			w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
