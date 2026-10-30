package middleware

import "net/http"

// SetSecurityHeaders applies the baseline headers on every response class.
func SetSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	// The SPA is a static adapter-static build: its hydration bootstrap is an
	// inline <script>, so 'unsafe-inline' is unavoidable without SSR nonces.
	// This policy still blocks every external origin, so an injected payload
	// cannot load or exfiltrate to an attacker host.
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' 'unsafe-inline'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob:; "+
			"font-src 'self' data:; "+
			"connect-src 'self'; "+
			"form-action 'self'; "+
			"object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
}

// maxRequestBody caps request bodies. docker-compose ships no reverse proxy,
// so nothing bounded uploads: any authenticated user could fill the disk.
const maxRequestBody = 10 << 20 // 10 MiB

// LimitBody caps request bodies before any handler reads them.
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		next.ServeHTTP(w, r)
	})
}
