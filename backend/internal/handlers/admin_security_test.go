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
	fuego.Post(s, "/users", h.Create)

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

// An admin-created account starts with a password the admin knows, so the
// server must force the user to replace it.
func TestCreateUser_FlagsForceChange(t *testing.T) {
	c := newAdminTestServer(t, uuid.New())

	if got := c.do(http.MethodPost, "/users", `{"email":"new@x.test","name":"New","role":"user","password":"TempPassw0rd!x"}`); got != http.StatusCreated {
		t.Fatalf("status = %d, want 201", got)
	}

	var flagged int
	testutil.NoErr(t, c.db.Raw(`SELECT force_password_change FROM admin_users WHERE email = ?`, "new@x.test").Scan(&flagged).Error)
	if flagged != 1 {
		t.Error("create user did not set force_password_change")
	}
}

// Self-registration is off unless the operator opts in.
func TestRegister_DisabledByDefault(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "")
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/register", NewAuthHandler(nil, nil).Register)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"email":"a@x.test","name":"A","password":"Passw0rd!x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
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

// POST /users must reject malformed emails, missing fields, and duplicates with
// a client error instead of creating a user or failing with an opaque 500.
func TestCreateUser_ValidatesInput(t *testing.T) {
	c := newAdminTestServer(t, uuid.New())

	if code := c.do(http.MethodPost, "/users", `{"email":"not-an-email","password":"Str0ngPassw0rd!","name":"A"}`); code != http.StatusBadRequest {
		t.Errorf("invalid email: status = %d, want 400", code)
	}
	if code := c.do(http.MethodPost, "/users", `{"email":"a@b.test","password":"Str0ngPassw0rd!","name":""}`); code != http.StatusBadRequest {
		t.Errorf("missing name: status = %d, want 400", code)
	}
	if code := c.do(http.MethodPost, "/users", `{"email":"dup@b.test","password":"Str0ngPassw0rd!","name":"A"}`); code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want 201", code)
	}
	if code := c.do(http.MethodPost, "/users", `{"email":"dup@b.test","password":"Str0ngPassw0rd!","name":"A"}`); code != http.StatusConflict {
		t.Errorf("duplicate email: status = %d, want 409", code)
	}
}

// "Name <addr>" is a valid RFC 5322 address; only the bare address may be
// stored, or logging in with it never matches.
func TestCreateUser_StoresBareAddress(t *testing.T) {
	c := newAdminTestServer(t, uuid.New())

	if got := c.do(http.MethodPost, "/users", `{"email":"Bob <Bob@X.test>","name":"Bob","role":"user","password":"TempPassw0rd!x"}`); got != http.StatusCreated {
		t.Fatalf("status = %d, want 201", got)
	}
	var email string
	testutil.NoErr(t, c.db.Raw(`SELECT email FROM admin_users`).Scan(&email).Error)
	if email != "bob@x.test" {
		t.Fatalf("stored email = %q, want bob@x.test", email)
	}
}

// A create whose wedding grants fail must not leave the user behind, and a
// malformed wedding ID is a client error, not silently dropped.
func TestCreateUser_WeddingGrantsAreAllOrNothing(t *testing.T) {
	c := newAdminTestServer(t, uuid.New()) // no user_weddings table: every grant insert fails
	body := func(wedding string) string {
		return `{"email":"g@x.test","name":"G","role":"user","password":"TempPassw0rd!x","weddings":["` + wedding + `"]}`
	}
	if got := c.do(http.MethodPost, "/users", body("not-an-id")); got != http.StatusBadRequest {
		t.Fatalf("malformed wedding ID: status = %d, want 400", got)
	}
	if got := c.do(http.MethodPost, "/users", body(uuid.New().String())); got < 500 {
		t.Fatalf("failing grant: status = %d, want 5xx", got)
	}
	var n int64
	testutil.NoErr(t, c.db.Raw(`SELECT COUNT(*) FROM admin_users WHERE email = 'g@x.test'`).Scan(&n).Error)
	if n != 0 {
		t.Fatalf("%d half-made users left behind", n)
	}
}
