package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveWithXFF(t *testing.T, trustedEnv, remoteAddr, xff string) string {
	t.Helper()
	t.Setenv("TRUSTED_PROXIES", trustedEnv)
	var got string
	h := ProxyAwareMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestProxyAware_XFFIgnoredWithoutTrust(t *testing.T) {
	if got := serveWithXFF(t, "", "203.0.113.7:1234", "6.6.6.6"); got != "203.0.113.7:1234" {
		t.Fatalf("spoofed XFF must be ignored when no trusted proxies, got %q", got)
	}
}

func TestProxyAware_TrustedProxyUsesClientHop(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1,10.0.0.0/8")
	var got string
	h := ProxyAwareMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 10.0.0.2")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "6.6.6.6:" {
		t.Fatalf("want client hop 6.6.6.6:, got %q", got)
	}
}

func TestProxyAware_UntrustedPeerIgnoresXFF(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1")
	var got string
	h := ProxyAwareMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.9:4444"
	req.Header.Set("X-Forwarded-For", "6.6.6.6")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.9:4444" {
		t.Fatalf("untrusted peer must not rewrite RemoteAddr, got %q", got)
	}
}
