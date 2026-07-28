package middleware

import (
	"net/http"
	"strings"
)

// ProxyAwareMiddleware extracts the real client IP from X-Forwarded-For
// header when behind a reverse proxy, updating RemoteAddr.
func ProxyAwareMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip, _, _ := strings.Cut(xff, ",")
			if ip != "" {
				r.RemoteAddr = strings.TrimSpace(ip)
			}
		}
		next.ServeHTTP(w, r)
	})
}
