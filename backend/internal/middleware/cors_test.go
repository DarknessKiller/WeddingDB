package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsResponse(t *testing.T, origin string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(rec, req)
	return rec
}

func TestCORS_ConfiguredOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://app.example.com")

	rec := corsResponse(t, "https://evil.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q, want the configured origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q, want true for the configured origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORS_UnconfiguredDoesNotReflectArbitraryOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")

	rec := corsResponse(t, "https://evil.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for an unconfigured origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORS_UnconfiguredAllowsLocalDevOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")

	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:3000"} {
		rec := corsResponse(t, origin)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("%s: Allow-Origin = %q, want echoed origin", origin, got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("%s: Allow-Credentials = %q, want empty without a configured origin", origin, got)
		}
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")

	rec := corsResponse(t, "")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty when the request has no Origin", got)
	}
}
