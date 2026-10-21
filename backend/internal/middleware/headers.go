package middleware

import "net/http"

// SetSecurityHeaders applies the baseline headers on every response class.
// The CSP is deliberately minimal (no script/style policy) so the SvelteKit
// SPA keeps working; tighten to a full policy only after browser-testing it.
func SetSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
}
