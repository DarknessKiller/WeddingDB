package tracing

import (
	"fmt"
	"net/http"
	"time"
)

// ServerTimingMiddleware wraps the handler chain and emits a Server-Timing
// header with the duration up to the first byte as "st" (e.g. "st;dur=12.3").
// Timing covers only downstream middlewares and the handler, since it
// is registered inside them.
func ServerTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&timingWriter{ResponseWriter: w, start: time.Now()}, r)
	})
}

// timingWriter sets the header before the response headers are flushed; a
// header set after the handler returns never reaches the client.
type timingWriter struct {
	http.ResponseWriter
	start time.Time
	wrote bool
}

func (w *timingWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	dur := float64(time.Since(w.start).Microseconds()) / 1000.0
	w.Header().Set("Server-Timing", fmt.Sprintf("st;dur=%.1f", dur))
	w.ResponseWriter.WriteHeader(code)
}

func (w *timingWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *timingWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *timingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
