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

func TestChangePassword_RequiresCurrentPassword(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, revoked INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	h := NewAuthHandler(authSvc, adminRepo)

	seed := func(force bool) uuid.UUID {
		id := uuid.New()
		hash, err := bcrypt.GenerateFromPassword([]byte("OldPassw0rd!"), bcrypt.MinCost)
		testutil.NoErr(t, err)
		testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role, force_password_change) VALUES (?, ?, ?, 'user', ?)`,
			id, id.String()+"@x.test", string(hash), force).Error)
		return id
	}
	do := func(id uuid.UUID, body string) int {
		s := fuego.NewServer(fuego.WithoutStartupMessages())
		fuego.Post(s, "/cp", h.ChangePassword)
		req := httptest.NewRequest(http.MethodPost, "/cp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), middleware.AdminIDKey, id))
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec.Code
	}

	normal := seed(false)
	if got := do(normal, `{"password":"NewPassw0rd!x"}`); got != http.StatusUnauthorized {
		t.Fatalf("missing currentPassword: got %d, want 401", got)
	}
	if got := do(normal, `{"currentPassword":"wrong","password":"NewPassw0rd!x"}`); got != http.StatusUnauthorized {
		t.Fatalf("wrong currentPassword: got %d, want 401", got)
	}
	if got := do(normal, `{"currentPassword":"OldPassw0rd!","password":"NewPassw0rd!x"}`); got != http.StatusOK {
		t.Fatalf("correct currentPassword: got %d, want 200", got)
	}
	forced := seed(true)
	if got := do(forced, `{"password":"NewPassw0rd!x"}`); got != http.StatusOK {
		t.Fatalf("forced change without currentPassword: got %d, want 200", got)
	}
}
