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

// Staff behind one venue NAT share an IP: successful logins must not use up
// the per-IP credential limit, only failed ones.
func TestLogin_PerIPLimitCountsOnlyFailures(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT, date DATETIME, deleted_at DATETIME)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), strings.Repeat("s", 32), rdb)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/login", NewAuthHandler(authSvc, adminRepo).Login)

	hash, err := bcrypt.GenerateFromPassword([]byte("Passw0rd!x"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, ?, 'admin')`,
		uuid.New(), "door@x.test", string(hash)).Error)

	login := func(password string) int {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"door@x.test","password":"`+password+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.9:1234"
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := range 11 {
		if got := login("Passw0rd!x"); got != http.StatusOK {
			t.Fatalf("successful login %d: got %d, want 200", i+1, got)
		}
	}
	for range 10 {
		if got := login("wrong"); got != http.StatusUnauthorized {
			t.Fatalf("wrong password: got %d, want 401", got)
		}
	}
	if got := login("Passw0rd!x"); got != http.StatusTooManyRequests {
		t.Fatalf("after 10 failures from one IP: got %d, want 429", got)
	}
}
