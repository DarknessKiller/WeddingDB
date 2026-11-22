package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func newAuthService(t *testing.T, secret string) (*services.AuthService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, token_version INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return services.NewAuthService(repository.NewAdminRepo(db), nil, nil, secret, rdb), db
}

func seedAdmin(t *testing.T, db *gorm.DB, id uuid.UUID) {
	t.Helper()
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, 'h', 'admin')`,
		id, id.String()+"@x.test").Error)
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	svc, _ := newAuthService(t, "secret")
	mw := AuthMiddleware(svc)
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
	svc, _ := newAuthService(t, "secret")
	mw := AuthMiddleware(svc)
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
	svc, _ := newAuthService(t, secret)
	mw := AuthMiddleware(svc)
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
	svc, db := newAuthService(t, secret)
	seedAdmin(t, db, adminID)
	mw := AuthMiddleware(svc)

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
	svc, _ := newAuthService(t, "secret")
	mw := AuthMiddleware(svc)
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

func TestAuthMiddleware_ForcePasswordChangeOnlyReachesChangePassword(t *testing.T) {
	secret := "test-secret"
	adminID := uuid.New()
	svc, db := newAuthService(t, secret)
	seedAdmin(t, db, adminID)
	mw := AuthMiddleware(svc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	claims := services.AccessClaims{
		AdminID:             adminID,
		Role:                "user",
		ForcePasswordChange: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	testutil.NoErr(t, err)

	tests := []struct {
		method, path string
		want         int
	}{
		{"POST", ChangePasswordPath, http.StatusOK},
		{"GET", "/api/weddings", http.StatusForbidden},
		{"POST", "/api/auth/select-wedding", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
