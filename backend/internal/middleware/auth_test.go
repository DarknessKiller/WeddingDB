package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"weddingdb/internal/services"
)

func signToken(secret string, adminID uuid.UUID, role string, wid *uuid.UUID, exp time.Time) string {
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
	s, _ := token.SignedString([]byte(secret))
	return s
}

func newAuthService(secret string) *services.AuthService {
	return services.NewAuthService(nil, nil, nil, secret)
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	mw := AuthMiddleware(newAuthService("secret"))
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
	mw := AuthMiddleware(newAuthService("secret"))
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
	mw := AuthMiddleware(newAuthService(secret))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tok := signToken(secret, uuid.New(), "admin", nil, time.Now().Add(-1*time.Hour))
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
	mw := AuthMiddleware(newAuthService(secret))

	var capturedAdminID uuid.UUID
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Context().Value(AdminIDKey); v != nil {
			capturedAdminID = v.(uuid.UUID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tok := signToken(secret, adminID, "admin", &wid, time.Now().Add(15*time.Minute))
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
	mw := AuthMiddleware(newAuthService("secret"))
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
