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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// closedDB is a gorm handle whose connections are already closed, so every
// query fails. It stands in for an infrastructure fault during revocation.
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())
	return db
}

// revocationFailureServer wires real handlers whose account writes succeed but
// whose refresh-token repository is down, so RevokeUserTokens always fails.
func revocationFailureServer(t *testing.T) (*fuego.Server, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	for _, stmt := range []string{
		`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
			name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0,
			token_version INTEGER NOT NULL DEFAULT 0, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
			token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT)`,
		`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT, created_at DATETIME)`,
		`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME, venue_name TEXT,
			venue_address TEXT, kiosk_description TEXT, kiosk_logo_url TEXT, kiosk_background_url TEXT,
			kiosk_background_blur INTEGER DEFAULT 0, kiosk_background_size TEXT, kiosk_background_pos_x TEXT,
			kiosk_background_pos_y TEXT, kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
			show_seat_numbers INTEGER DEFAULT 1, kiosk_idle_reset_seconds INTEGER DEFAULT 0, hall_width INTEGER,
			hall_height INTEGER, layout_version INTEGER NOT NULL DEFAULT 0, created_at DATETIME, updated_at DATETIME)`,
	} {
		testutil.NoErr(t, db.Exec(stmt).Error)
	}

	caller, target, wedding := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role) VALUES (?, ?, 'h', 'admin')`,
		caller, caller.String()+"@x.test").Error)
	testutil.NoErr(t, db.Exec(`INSERT INTO admin_users (id, email, password, role, force_password_change) VALUES (?, ?, 'h', 'user', 1)`,
		target, target.String()+"@x.test").Error)
	testutil.NoErr(t, db.Exec(`INSERT INTO wedding_events (id, name) VALUES (?, 'Test Wedding')`, wedding).Error)
	testutil.NoErr(t, db.Exec(`INSERT INTO user_weddings (id, user_id, wedding_id) VALUES (?, ?, ?)`,
		uuid.New(), target, wedding).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(closedDB(t)), "secret", rdb)
	adminHandler := NewAdminHandler(adminRepo, authSvc, services.NewWeddingService(repository.NewWeddingRepo(db)))
	authHandler := NewAuthHandler(authSvc, adminRepo)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Delete(s, "/users/{id}", adminHandler.Delete)
	fuego.Put(s, "/users/{id}/weddings", adminHandler.AssignWeddings)
	fuego.Post(s, "/users/{id}/reset-password", adminHandler.ResetPassword)
	fuego.Put(s, "/users/{id}/role", adminHandler.UpdateRole)
	fuego.Post(s, "/change-password", authHandler.ChangePassword)
	return s, caller, target
}

func revocationRequest(t *testing.T, s *fuego.Server, method, path, caller string, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.AdminIDKey, uuid.MustParse(caller))
	ctx = context.WithValue(ctx, middleware.RoleKey, "admin")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req.WithContext(ctx))
	return rec.Code
}

func TestRevokeFailureIsReported(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   func(caller, target uuid.UUID) string
		body   string
	}{
		{"delete user", http.MethodDelete, func(_, target uuid.UUID) string {
			return "/users/" + target.String()
		}, ""},
		{"assign weddings", http.MethodPut, func(_, target uuid.UUID) string {
			return "/users/" + target.String() + "/weddings"
		}, `{"weddings":[]}`},
		{"reset password", http.MethodPost, func(_, target uuid.UUID) string {
			return "/users/" + target.String() + "/reset-password"
		}, `{"password":"NewPassw0rd!"}`},
		{"update role", http.MethodPut, func(_, target uuid.UUID) string {
			return "/users/" + target.String() + "/role"
		}, `{"role":"user"}`},
		{"change own password", http.MethodPost, func(_, _ uuid.UUID) string {
			return "/change-password"
		}, `{"password":"NewPassw0rd!"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, caller, target := revocationFailureServer(t)
			contextID := caller
			if tt.name == "change own password" {
				// The target row has ForcePasswordChange set, so no current password is needed.
				contextID = target
			}
			got := revocationRequest(t, s, tt.method, tt.path(caller, target), contextID.String(), tt.body)
			if got != http.StatusInternalServerError {
				t.Fatalf("revocation failure returned %d, want 500 (success would hide an active session)", got)
			}
		})
	}
}
