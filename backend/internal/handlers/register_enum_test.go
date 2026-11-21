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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A taken email must get the same answer as a new one, or register is an
// oracle for which emails have accounts.
func TestRegister_TakenEmailLooksLikeNew(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, token_version INTEGER NOT NULL DEFAULT 0, revoked INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/register", NewAuthHandler(authSvc, adminRepo).Register)

	do := func() (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"email":"a@x.test","name":"A","password":"Passw0rd!x1"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	newCode, newBody := do()
	if newCode != http.StatusCreated {
		t.Fatalf("new email: got %d %s, want 201", newCode, newBody)
	}
	takenCode, takenBody := do()
	if takenCode != newCode || takenBody != newBody {
		t.Fatalf("taken email answered %d %q, new email %d %q: must match", takenCode, takenBody, newCode, newBody)
	}
}
