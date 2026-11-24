package tracing

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerTimingHeaderBeforeFirstWrite(t *testing.T) {
	for name, handler := range map[string]http.Handler{
		"explicit status": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, "ok") //nolint:errcheck
		}),
		"implicit status": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "ok") //nolint:errcheck
		}),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(ServerTimingMiddleware(handler))
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close() //nolint:errcheck // test cleanup
			if got := resp.Header.Get("Server-Timing"); got == "" {
				t.Fatal("Server-Timing header missing from client response")
			}
		})
	}
}
