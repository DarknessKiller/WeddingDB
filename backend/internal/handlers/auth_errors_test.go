package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A DB failure during login is a 500 with a fixed title, not a 401 that leaks
// the driver error and counts against the account's lockout.
func TestLogin_InternalErrorIs500NotLeaked(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	// No admin_users table: FindByEmail fails with a driver error.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/login", NewAuthHandler(authSvc, adminRepo).Login)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"a@x.test","password":"whatever1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "no such table") {
		t.Fatalf("driver error leaked: %s", rec.Body.String())
	}
	for _, k := range mr.Keys() {
		if strings.HasPrefix(k, "login:acct:") {
			t.Fatalf("internal error recorded as login failure: %s", k)
		}
	}
}

// Wrong current passwords share Login's per-account lockout.
func TestChangePassword_RateLimitsCurrentPassword(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/cp", NewAuthHandler(authSvc, adminRepo).ChangePassword)

	id := uuid.New()
	hash, err := bcrypt.GenerateFromPassword([]byte("OldPassw0rd!"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, ?, 'user')`,
		id, "cp@x.test", string(hash)).Error)

	do := func(current string) int {
		req := httptest.NewRequest(http.MethodPost, "/cp",
			strings.NewReader(`{"currentPassword":"`+current+`","password":"NewPassw0rd!x"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), middleware.AdminIDKey, id))
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec.Code
	}
	got := 0
	for range 100 {
		if got = do("wrong"); got != http.StatusUnauthorized {
			break
		}
	}
	if got != http.StatusTooManyRequests {
		t.Fatalf("after repeated wrong current passwords: got %d, want 429", got)
	}
	if got := do("OldPassw0rd!"); got != http.StatusTooManyRequests {
		t.Fatalf("locked account with correct password: got %d, want 429", got)
	}
}
