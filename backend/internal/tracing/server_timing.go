package tracing

import (
	"fmt"
	"net/http"
	"time"
)

// ServerTimingMiddleware emits a Server-Timing header with the total request
// duration as "st" (e.g. "st;dur=12.3"). The header must be set before the
// handler writes its status line, so the writer is wrapped instead of setting
// it after ServeHTTP returns.
func ServerTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &timingWriter{ResponseWriter: w, start: time.Now()}
		next.ServeHTTP(tw, r)
	})
}

type timingWriter struct {
	http.ResponseWriter
	start time.Time
	wrote bool
}

func (tw *timingWriter) setHeader() {
	dur := float64(time.Since(tw.start).Microseconds()) / 1000.0
	tw.Header().Set("Server-Timing", fmt.Sprintf("st;dur=%.1f", dur))
}

func (tw *timingWriter) WriteHeader(code int) {
	if !tw.wrote {
		tw.wrote = true
		tw.setHeader()
	}
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timingWriter) Write(b []byte) (int, error) {
	if !tw.wrote {
		tw.wrote = true
		tw.setHeader()
	}
	return tw.ResponseWriter.Write(b)
}

// Flush keeps SSE handlers working: they type-assert the writer to http.Flusher.
func (tw *timingWriter) Flush() {
	if f, ok := tw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (tw *timingWriter) Unwrap() http.ResponseWriter { return tw.ResponseWriter }
