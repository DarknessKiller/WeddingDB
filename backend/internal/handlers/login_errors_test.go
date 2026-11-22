package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// loginServer wires a real Login handler over the given DB.
func loginServer(t *testing.T, db *gorm.DB) *fuego.Server {
	t.Helper()
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	h := NewAuthHandler(authSvc, adminRepo)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/login", h.Login)
	return s
}

func loginStatus(t *testing.T, s *fuego.Server, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestLogin_InvalidCredentialsIs401(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, token_version INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	hash, err := bcrypt.GenerateFromPassword([]byte("CorrectPass1!"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, 'a@x.test', ?, 'admin')`,
		"7b2f6c1e-0000-4000-8000-000000000001", string(hash)).Error)
	s := loginServer(t, db)

	if got := loginStatus(t, s, `{"email":"a@x.test","password":"WrongPass1!"}`); got != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d, want 401", got)
	}
	if got := loginStatus(t, s, `{"email":"nobody@x.test","password":"WrongPass1!"}`); got != http.StatusUnauthorized {
		t.Fatalf("unknown email: got %d, want 401", got)
	}
}

// A database fault must not masquerade as bad credentials.
func TestLogin_InfrastructureErrorIs503(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	// No admin_users table: the lookup fails before any credential check.
	s := loginServer(t, db)

	if got := loginStatus(t, s, `{"email":"a@x.test","password":"CorrectPass1!"}`); got != http.StatusServiceUnavailable {
		t.Fatalf("infrastructure failure: got %d, want 503", got)
	}
}
