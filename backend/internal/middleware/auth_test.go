package middleware

import (
	"net/http"
	"net/http/httptest"
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

// newAuthService has no database, so it pre-caches token version 0 for users.
func newAuthService(t *testing.T, secret string, users ...uuid.UUID) *services.AuthService {
	t.Helper()
	mr := miniredis.RunT(t)
	for _, id := range users {
		testutil.NoErr(t, mr.Set("user:"+id.String()+":tv", "0"))
	}
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
	mw := AuthMiddleware(newAuthService(t, secret, adminID))

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

// A Redis outage must not read as an expired session: every client would rotate
// its tokens into the same outage and the storm would keep it down.
func TestAuthMiddleware_RevocationCheckOutageIs503(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := services.NewAuthService(nil, nil, nil, "secret", rdb)
	mr.Close()

	tok := signToken(t, "secret", uuid.New(), "admin", nil, time.Now().Add(time.Hour))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/weddings", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	AuthMiddleware(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
