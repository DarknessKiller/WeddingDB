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

// adminClient wires an AdminHandler over in-memory SQLite + miniredis and
// returns a do(method, path, body) that runs a request as the given admin.
type adminClient struct {
	do   func(method, path, body string) int
	db   *gorm.DB
	auth *services.AuthService
}

func newAdminTestServer(t *testing.T, adminID uuid.UUID) *adminClient {
	t.Helper()
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
	h := NewAdminHandler(adminRepo, authSvc)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Delete(s, "/users/{id}", h.Delete)
	fuego.Post(s, "/users/{id}/reset-password", h.ResetPassword)

	// Same identity injection as AuthMiddleware.
	adminCtx := context.WithValue(context.Background(), middleware.AdminIDKey, adminID)
	adminCtx = context.WithValue(adminCtx, middleware.RoleKey, "admin")

	return &adminClient{
		db:   db,
		auth: authSvc,
		do: func(method, path, body string) int {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(adminCtx)
			rec := httptest.NewRecorder()
			s.Mux.ServeHTTP(rec, req)
			return rec.Code
		},
	}
}

func seedUser(t *testing.T, db *gorm.DB, role string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	hash, err := bcrypt.GenerateFromPassword([]byte("OldPassw0rd!"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, ?, ?)`,
		id, id.String()+"@x.test", string(hash), role).Error)
	return id
}

// Admin password reset must clear out the account's sessions too, and flag the
// user so the server (not just the browser) forces them to pick a new password.
func TestResetPassword_FlagsForceChange(t *testing.T) {
	c := newAdminTestServer(t, uuid.New())
	user := seedUser(t, c.db, "user")

	if got := c.do(http.MethodPost, "/users/"+user.String()+"/reset-password", `{"password":"TempPassw0rd!x"}`); got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}

	var flagged int
	testutil.NoErr(t, c.db.Raw(`SELECT force_password_change FROM admin_users WHERE id = ?`, user).Scan(&flagged).Error)
	if flagged != 1 {
		t.Error("reset password did not set force_password_change")
	}
}

// Deleting a user must revoke their tokens before the row disappears, since
// ValidateToken never checks that the user still exists.
func TestDeleteUser_RevokesTokens(t *testing.T) {
	c := newAdminTestServer(t, uuid.New())
	user := seedUser(t, c.db, "user")
	testutil.NoErr(t, c.db.Exec(`INSERT INTO refresh_tokens (id, admin_id, token) VALUES (?, ?, ?)`,
		uuid.New(), user, "refresh-secret").Error)
	before, err := c.auth.GetTokenVersion(t.Context(), user)
	testutil.NoErr(t, err)

	if got := c.do(http.MethodDelete, "/users/"+user.String(), ""); got != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", got)
	}

	var tokens int
	testutil.NoErr(t, c.db.Raw(`SELECT COUNT(*) FROM refresh_tokens WHERE admin_id = ?`, user).Scan(&tokens).Error)
	if tokens != 0 {
		t.Errorf("%d refresh tokens survived the delete", tokens)
	}
	after, err := c.auth.GetTokenVersion(t.Context(), user)
	testutil.NoErr(t, err)
	if after <= before {
		t.Errorf("token version %d did not increase (was %d)", after, before)
	}
}
