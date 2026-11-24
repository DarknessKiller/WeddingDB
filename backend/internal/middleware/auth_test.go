package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func signToken(t *testing.T, secret string, adminID uuid.UUID, role string, wid *uuid.UUID, exp time.Time) string {
	now := time.Now()
	claims := services.AccessClaims{
		AdminID:   adminID,
		WeddingID: wid,
		Role:      role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	testutil.NoErr(t, err)
	return s
}

func newAuthService(t *testing.T, secret string) *services.AuthService {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return services.NewAuthService(nil, nil, nil, secret, rdb)
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	mw := AuthMiddleware(newAuthService(t, "secret"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/weddings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	mw := AuthMiddleware(newAuthService(t, "secret"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.value")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	mw := AuthMiddleware(newAuthService(t, secret))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tok := signToken(t, secret, uuid.New(), "admin", nil, time.Now().Add(-1*time.Hour))
	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret"
	adminID := uuid.New()
	wid := uuid.New()
	mw := AuthMiddleware(newAuthService(t, secret))

	var capturedAdminID uuid.UUID
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Context().Value(AdminIDKey); v != nil {
			capturedAdminID = v.(uuid.UUID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tok := signToken(t, secret, adminID, "admin", &wid, time.Now().Add(15*time.Minute))
	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedAdminID != adminID {
		t.Errorf("context AdminID = %v, want %v", capturedAdminID, adminID)
	}
}

func TestAuthMiddleware_WrongBearerFormat(t *testing.T) {
	mw := AuthMiddleware(newAuthService(t, "secret"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Token abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// A Redis outage is not a bad token: 503 keeps the client's session, and no
// driver text reaches the body.
func TestAuthMiddleware_RedisDownIs503(t *testing.T) {
	secret := "test-secret"
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mw := AuthMiddleware(services.NewAuthService(nil, nil, nil, secret, rdb))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mr.Close()

	tok := signToken(t, secret, uuid.New(), "admin", nil, time.Now().Add(15*time.Minute))
	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); strings.Contains(body, "dial") || strings.Contains(body, "connection") {
		t.Errorf("body leaks driver text: %q", body)
	}
}

// A session flagged for a forced password change reaches change-password and
// nothing else.
func TestAuthMiddleware_ForcePasswordChangeBlocksOtherRoutes(t *testing.T) {
	secret := "test-secret"
	mw := AuthMiddleware(newAuthService(t, secret))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, services.AccessClaims{
		AdminID: uuid.New(), Role: "user", ForcePasswordChange: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ID: uuid.New().String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}).SignedString([]byte(secret))
	testutil.NoErr(t, err)

	for path, want := range map[string]int{
		"/api/weddings":             http.StatusForbidden,
		"/api/auth/select-wedding":  http.StatusForbidden,
		"/api/auth/change-password": http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s status = %d, want %d", path, rec.Code, want)
		}
	}
}
