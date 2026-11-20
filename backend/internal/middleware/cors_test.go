package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveCORS(t *testing.T, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/api/weddings", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// An unset CORS_ORIGIN must not reflect a caller-supplied Origin: that grants
// any website read access to authenticated API responses.
func TestCORS_UnsetOriginDoesNotReflect(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")
	rec := serveCORS(t, http.MethodGet, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want empty", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

// Only the exact configured origin is allowed, and only it gets credentials.
func TestCORS_AllowsConfiguredOriginOnly(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://app.example")

	rec := serveCORS(t, http.MethodGet, "https://app.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("matching origin: Access-Control-Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("matching origin: Access-Control-Allow-Credentials = %q, want true", got)
	}

	rec = serveCORS(t, http.MethodGet, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("foreign origin: Access-Control-Allow-Origin = %q, want empty", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("foreign origin: Access-Control-Allow-Credentials = %q, want empty", got)
	}
}

// A preflight from a foreign origin must not advertise an allowed origin.
func TestCORS_PreflightFromForeignOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://app.example")
	rec := serveCORS(t, http.MethodOptions, "https://evil.example")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

// Every method the API routes use must pass the preflight: layout save is PATCH.
func TestCORS_AllowsEveryRouteMethod(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://app.example")
	rec := serveCORS(t, http.MethodOptions, "https://app.example")
	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !strings.Contains(allowed, m) {
			t.Errorf("Access-Control-Allow-Methods = %q, missing %s", allowed, m)
		}
	}
}
