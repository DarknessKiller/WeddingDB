package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newScopeEnv(t *testing.T) (*repository.AdminRepo, *services.WeddingService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	// Raw DDL: the model's uuid default is Postgres-only.
	stmts := []string{
		`CREATE TABLE wedding_events (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			date DATETIME,
			venue_name TEXT,
			venue_address TEXT,
			kiosk_description TEXT,
			kiosk_logo_url TEXT,
			kiosk_background_url TEXT,
			kiosk_background_blur INTEGER,
			kiosk_background_size TEXT,
			kiosk_background_pos_x TEXT,
			kiosk_background_pos_y TEXT,
			kiosk_logo_size TEXT,
			kiosk_logo_pos_x TEXT,
			kiosk_logo_pos_y TEXT,
			show_seat_numbers NUMERIC DEFAULT false,
			kiosk_idle_reset_seconds INTEGER NOT NULL DEFAULT 0,
			hall_width INTEGER NOT NULL DEFAULT 860,
			hall_height INTEGER NOT NULL DEFAULT 1000,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE user_weddings (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			wedding_id TEXT NOT NULL,
			created_at DATETIME
		)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	return repository.NewAdminRepo(db), services.NewWeddingService(repository.NewWeddingRepo(db)), db
}

func serveScoped(t *testing.T, mw func(http.Handler) http.Handler, role string, adminID, jwtWid, urlWid uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/weddings/x/guests", nil)
	req.SetPathValue("wid", urlWid.String())
	ctx := context.WithValue(req.Context(), RoleKey, role)
	ctx = context.WithValue(ctx, AdminIDKey, adminID)
	ctx = context.WithValue(ctx, WeddingIDKey, &jwtWid)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// A user whose assignment was revoked must not keep using a token issued while
// they still had access.
func TestWeddingScope_RevokedAssignmentDenied(t *testing.T) {
	adminRepo, weddingService, db := newScopeEnv(t)
	wid := uuid.New()
	userID := uuid.New()
	if err := db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}
	if err := db.Create(&models.UserWedding{ID: uuid.New(), UserID: userID, WeddingID: wid}).Error; err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	mw := WeddingScopeMiddleware(adminRepo, weddingService)

	if rec := serveScoped(t, mw, "staff", userID, wid, wid); rec.Code != http.StatusNoContent {
		t.Fatalf("assigned user status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if err := db.Where("user_id = ? AND wedding_id = ?", userID, wid).Delete(&models.UserWedding{}).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if rec := serveScoped(t, mw, "staff", userID, wid, wid); rec.Code != http.StatusForbidden {
		t.Fatalf("revoked user status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// An admin token must stop working on a wedding that has been deleted.
func TestWeddingScope_DeletedWeddingDeniedForAdmin(t *testing.T) {
	adminRepo, weddingService, db := newScopeEnv(t)
	wid := uuid.New()
	adminID := uuid.New()
	if err := db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}
	mw := WeddingScopeMiddleware(adminRepo, weddingService)

	if rec := serveScoped(t, mw, "admin", adminID, wid, wid); rec.Code != http.StatusNoContent {
		t.Fatalf("admin status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if err := db.Delete(&models.WeddingEvent{}, "id = ?", wid).Error; err != nil {
		t.Fatalf("delete wedding: %v", err)
	}
	if rec := serveScoped(t, mw, "admin", adminID, wid, wid); rec.Code != http.StatusForbidden {
		t.Fatalf("deleted wedding status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
