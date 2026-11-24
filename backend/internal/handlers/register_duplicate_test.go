package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-fuego/fuego"
	"github.com/redis/go-redis/v9"
)

// Two registrations with the same email race past the lookup and hit the unique
// index; the loser must be 409, not 500.
func TestRegister_ConcurrentDuplicateEmailIs409(t *testing.T) {
	db := newSQLite(t, adminUsersDDL,
		`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT, wedding_id TEXT,
			token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME,
			rotated_at DATETIME, replaced_by TEXT, token_version INTEGER DEFAULT 0)`,
		userWeddingsDDL)
	testutil.NoErr(t, db.Exec("CREATE UNIQUE INDEX idx_admin_users_email ON admin_users (email)").Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })

	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	h := NewAuthHandler(authSvc, adminRepo)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/register", h.Register)

	body := `{"email":"race@example.com","password":"NewPassw0rd!","name":"Race"}`
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Go(func() {
			req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.Mux.ServeHTTP(rec, req)
			codes[i] = rec.Code
		})
	}
	wg.Wait()

	want := map[int]int{http.StatusCreated: 1, http.StatusConflict: 1}
	got := map[int]int{}
	for _, code := range codes {
		got[code]++
	}
	for code, n := range want {
		if got[code] != n {
			t.Fatalf("statuses = %v, want one 201 and one 409", codes)
		}
	}
}
