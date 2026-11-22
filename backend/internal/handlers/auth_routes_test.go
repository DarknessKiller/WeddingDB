package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/config"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type authRoutes struct {
	s      http.Handler
	db     *gorm.DB
	mr     *miniredis.Miniredis
	secret string
}

func newAuthRoutes(t *testing.T) *authRoutes {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	for _, stmt := range []string{
		`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT, password TEXT, name TEXT, role TEXT,
			force_password_change INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT, created_at DATETIME)`,
		`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT, token_version INTEGER DEFAULT 0,
			wedding_id TEXT, token TEXT UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`,
	} {
		testutil.NoErr(t, db.Exec(stmt).Error)
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	const secret = "test-secret"
	adminRepo := repository.NewAdminRepo(db)
	auth := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), secret, rdb)
	s := config.NewFuegoServer(config.Env{Port: "0"}, "test")
	// Guest service without repositories: enough for paths answered before any query.
	RegisterRoutes(s, auth, services.NewGuestService(nil, nil, nil, nil), nil, nil, nil, nil, adminRepo)
	return &authRoutes{s: s.Mux, db: db, mr: mr, secret: secret}
}

func (a *authRoutes) token(t *testing.T, id uuid.UUID, role string) string {
	t.Helper()
	claims := services.AccessClaims{AdminID: id, Role: role, RegisteredClaims: jwt.RegisteredClaims{
		ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.secret))
	testutil.NoErr(t, err)
	return tok
}

func (a *authRoutes) post(path, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	a.s.ServeHTTP(rec, req)
	return rec
}

// A rejected token is 401; a Redis or DB outage is 503 so the client keeps
// its session. Covers the middleware and the Refresh handler.
func TestRegisteredAuthRejectionVsOutage(t *testing.T) {
	a := newAuthRoutes(t)
	body := `{"weddingId":"` + utils.EncodeUUID(uuid.New()) + `"}`

	if rec := a.post("/api/auth/select-wedding", "garbage", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad bearer: status %d, want 401", rec.Code)
	}
	if rec := a.post("/api/auth/refresh", "", `{"refreshToken":"unknown"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown refresh: status %d, want 401, body %s", rec.Code, rec.Body.String())
	}

	sqlDB, err := a.db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())
	rec := a.post("/api/auth/refresh", "", `{"refreshToken":"unknown"}`)
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "database is closed") {
		t.Fatalf("refresh during DB outage: status %d, body %s", rec.Code, rec.Body.String())
	}

	tok := a.token(t, uuid.New(), "admin")
	a.mr.Close()
	rec = a.post("/api/auth/select-wedding", tok, body)
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "dial") {
		t.Fatalf("protected route during Redis outage: status %d, body %s", rec.Code, rec.Body.String())
	}
}

// select-wedding rescopes only the refresh token named in the body; the same
// user's other session keeps its scope.
func TestRegisteredSelectWeddingScopesOnlyCallerSession(t *testing.T) {
	a := newAuthRoutes(t)
	uid, wid := uuid.New(), uuid.New()
	testutil.NoErr(t, a.db.Create(&models.AdminUser{ID: uid, Email: "u@x.test", Role: "user"}).Error)
	testutil.NoErr(t, a.db.Create(&models.UserWedding{ID: uuid.New(), UserID: uid, WeddingID: wid}).Error)
	exp := time.Now().Add(time.Hour)
	for _, tok := range []string{"mine", "other-device"} {
		testutil.NoErr(t, a.db.Create(&models.RefreshToken{ID: uuid.New(), AdminID: uid, Token: tok, ExpiresAt: exp}).Error)
	}

	rec := a.post("/api/auth/select-wedding", a.token(t, uid, "user"),
		`{"weddingId":"`+utils.EncodeUUID(wid)+`","refreshToken":"mine"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	scope := func(tok string) *uuid.UUID {
		var rt models.RefreshToken
		testutil.NoErr(t, a.db.Where("token = ?", tok).First(&rt).Error)
		return rt.WeddingID
	}
	if got := scope("mine"); got == nil || *got != wid {
		t.Fatalf("caller session scope = %v, want %s", got, wid)
	}
	if got := scope("other-device"); got != nil {
		t.Fatalf("other session rescoped to %s", *got)
	}
}
