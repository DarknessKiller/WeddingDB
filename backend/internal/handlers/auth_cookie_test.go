package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenResponse_OmitsRefreshToken(t *testing.T) {
	// The whole point of the cookie: the refresh token must never travel in a
	// response body where page script can read it.
	b, err := json.Marshal(TokenResponse{AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "refreshToken") || strings.Contains(string(b), "refresh_token") {
		t.Fatalf("TokenResponse still serializes a refresh token: %s", b)
	}
}

func TestRefreshCookie_Hardened(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	c := refreshCookie(r, "tok")

	if !c.HttpOnly {
		t.Error("HttpOnly is false: page script could read the refresh token")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict (CSRF protection)", c.SameSite)
	}
	if c.Path != "/api/auth" {
		t.Errorf("Path = %q, want /api/auth", c.Path)
	}
	if c.Secure {
		t.Error("Secure set for a plain-HTTP request; the cookie would never be stored")
	}
}

func TestRefreshCookie_SecureBehindTLSProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if !refreshCookie(r, "tok").Secure {
		t.Error("Secure not set behind an HTTPS proxy")
	}
}

func TestClearedRefreshCookie_ExpiresImmediately(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	if got := clearedRefreshCookie(r).MaxAge; got >= 0 {
		t.Errorf("MaxAge = %d, want negative to evict the cookie", got)
	}
}

func TestRefreshTokenFrom(t *testing.T) {
	withCookie := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	withCookie.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "from-cookie"})

	if got := refreshTokenFrom(withCookie, ""); got != "from-cookie" {
		t.Errorf("cookie not used: got %q", got)
	}
	if got := refreshTokenFrom(withCookie, "from-body"); got != "from-body" {
		t.Errorf("body should still work for non-browser clients: got %q", got)
	}
	bare := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	if got := refreshTokenFrom(bare, ""); got != "" {
		t.Errorf("expected empty token, got %q", got)
	}
}
