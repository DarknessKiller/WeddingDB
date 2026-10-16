package tracing

import (
	"fmt"
	"net/http"
	"time"
)

// Middleware wraps the handler chain and emits a Server-Timing header
// with the total request duration as "st" (e.g. "st;dur=12.3").
// Timing covers only downstream middlewares and the handler, since it
// is registered inside them.
func ServerTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)

		dur := float64(time.Since(start).Microseconds()) / 1000.0
		w.Header().Set("Server-Timing", fmt.Sprintf("st;dur=%.1f", dur))
	})
}
