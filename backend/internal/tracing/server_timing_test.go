package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerTimingHeaderIsSent(t *testing.T) {
	h := ServerTimingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Result().Header.Get("Server-Timing"); got == "" {
		t.Fatal("Server-Timing header missing")
	}
}
