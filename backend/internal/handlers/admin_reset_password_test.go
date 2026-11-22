package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
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

// TestAdminResetPassword_ForcesChange pins that an admin-chosen password is
// only usable until the target picks their own.
func TestAdminResetPassword_ForcesChange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, revoked INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT, token_version INTEGER DEFAULT 0)`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })
	adminRepo := repository.NewAdminRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), repository.NewTokenRepo(db), "secret", rdb)
	h := NewAdminHandler(adminRepo, authSvc)

	callerID, targetID := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: callerID, Email: callerID.String() + "@x.test", Password: "x", Role: "admin"}).Error)
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: targetID, Email: targetID.String() + "@x.test", Password: "x", Role: "user"}).Error)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetID.String()+"/reset-password",
		strings.NewReader(`{"password":"NewPassw0rd!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", targetID.String())
	ctx := context.WithValue(req.Context(), middleware.AdminIDKey, callerID)
	ctx = context.WithValue(ctx, middleware.RoleKey, "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	fuego.HTTPHandler[any, ResetPasswordRequest, any](s, h.ResetPassword,
		fuego.BaseRoute{Path: "/admin/users/{id}/reset-password", Method: http.MethodPost})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var forced bool
	testutil.NoErr(t, db.Raw(`SELECT force_password_change FROM admin_users WHERE id = ?`, targetID).Scan(&forced).Error)
	if !forced {
		t.Fatal("force_password_change = false after admin reset, want true")
	}
}
