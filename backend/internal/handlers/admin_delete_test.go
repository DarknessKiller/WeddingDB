package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestAdminDelete_RevokesSessionsAndGrants pins the delete contract: the
// account row, its wedding grants and its live sessions all go together.
func TestAdminDelete_RevokesSessionsAndGrants(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	// ":memory:" gives every new connection its own empty database, so keep one.
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	testutil.NoErr(t, db.Exec(`CREATE TABLE admin_users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
		name TEXT, role TEXT NOT NULL, force_password_change INTEGER DEFAULT 0, revoked INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE refresh_tokens (id TEXT PRIMARY KEY, admin_id TEXT NOT NULL, wedding_id TEXT,
		token TEXT NOT NULL UNIQUE, expires_at DATETIME, created_at DATETIME, rotated_at DATETIME, replaced_by TEXT, token_version INTEGER DEFAULT 0)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME,
		venue_name TEXT, venue_address TEXT, kiosk_description TEXT, kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER DEFAULT 0, kiosk_background_size TEXT, kiosk_background_pos_x TEXT,
		kiosk_background_pos_y TEXT, kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER DEFAULT 1, kiosk_idle_reset_seconds INTEGER DEFAULT 0, hall_width INTEGER DEFAULT 860,
		hall_height INTEGER DEFAULT 1000, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, user_id TEXT NOT NULL,
		wedding_id TEXT NOT NULL, created_at DATETIME, UNIQUE(user_id, wedding_id))`).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { testutil.NoErr(t, rdb.Close()) })

	adminRepo := repository.NewAdminRepo(db)
	tokenRepo := repository.NewTokenRepo(db)
	authSvc := services.NewAuthService(adminRepo, repository.NewWeddingRepo(db), tokenRepo, "secret", rdb)
	h := NewAdminHandler(adminRepo, authSvc)

	callerID, targetID, weddingID := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: callerID, Email: callerID.String() + "@x.test", Password: "x", Role: "admin"}).Error)
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: targetID, Email: targetID.String() + "@x.test", Password: "x", Role: "user"}).Error)
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: weddingID, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.UserWedding{ID: uuid.New(), UserID: targetID, WeddingID: weddingID}).Error)
	testutil.NoErr(t, db.Create(&models.RefreshToken{
		ID: uuid.New(), AdminID: targetID, WeddingID: &weddingID, Token: "refresh-" + targetID.String(),
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+targetID.String(), nil)
	req.SetPathValue("id", targetID.String())
	ctx := context.WithValue(req.Context(), middleware.AdminIDKey, callerID)
	ctx = context.WithValue(ctx, middleware.RoleKey, "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	fuego.HTTPHandler[any, any, any](s, h.Delete, fuego.BaseRoute{Path: "/admin/users/{id}", Method: http.MethodDelete})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	var grants, tokens int64
	testutil.NoErr(t, db.Model(&models.UserWedding{}).Where("user_id = ?", targetID).Count(&grants).Error)
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Where("admin_id = ?", targetID).Count(&tokens).Error)
	if grants != 0 {
		t.Fatalf("user_weddings rows left behind: %d, want 0", grants)
	}
	if tokens != 0 {
		t.Fatalf("refresh_tokens rows left behind: %d, want 0", tokens)
	}

	tv, err := authSvc.GetTokenVersion(context.Background(), targetID)
	testutil.NoErr(t, err)
	if tv != 1 {
		t.Fatalf("token version = %d, want 1", tv)
	}
}
