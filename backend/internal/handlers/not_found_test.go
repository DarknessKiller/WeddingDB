package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const weddingEventsDDL = `CREATE TABLE wedding_events (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME,
	venue_name TEXT, venue_address TEXT, kiosk_description TEXT,
	kiosk_logo_url TEXT, kiosk_background_url TEXT,
	kiosk_background_blur INTEGER DEFAULT 0, kiosk_background_size TEXT,
	kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
	kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
	show_seat_numbers INTEGER DEFAULT 1, kiosk_idle_reset_seconds INTEGER DEFAULT 0,
	hall_width INTEGER DEFAULT 860, hall_height INTEGER DEFAULT 1000,
	created_at DATETIME, updated_at DATETIME)`

func newSQLite(t *testing.T, ddl ...string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	for _, stmt := range ddl {
		testutil.NoErr(t, db.Exec(stmt).Error)
	}
	return db
}

func TestWeddingGet_MissingIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL)
	h := NewWeddingHandler(services.NewWeddingService(repository.NewWeddingRepo(db)), repository.NewAdminRepo(db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/weddings/{id}", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/weddings/"+utils.EncodeUUID(uuid.New()), nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RoleKey, "admin"))
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body.String())
	}
}

func TestWeddingDelete_MissingIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL,
		`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, wedding_id TEXT)`,
	)
	h := NewWeddingHandler(services.NewWeddingService(repository.NewWeddingRepo(db)), repository.NewAdminRepo(db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Delete(s, "/api/weddings/{id}", h.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/weddings/"+utils.EncodeUUID(uuid.New()), nil)
	req = withAdmin(req)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a missing wedding, body %s", rec.Code, rec.Body.String())
	}
}

func TestLayoutGet_MissingWeddingIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL,
		`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT, name TEXT, capacity INTEGER,
			x REAL DEFAULT 0, y REAL DEFAULT 0, degree REAL DEFAULT 0, is_vip INTEGER,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT, type TEXT, x REAL, y REAL,
			degree REAL, width REAL, height REAL, name TEXT, color TEXT, text_color TEXT,
			stroke_color TEXT, opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`,
	)
	h := NewLayoutHandler(
		services.NewLayoutService(repository.NewLayoutRepo(db)),
		services.NewTableService(repository.NewTableRepo(db), nil, db),
		services.NewWeddingService(repository.NewWeddingRepo(db)),
	)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/weddings/{wid}/layout", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/weddings/"+utils.EncodeUUID(uuid.New())+"/layout", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body.String())
	}
}

func TestReport_MissingWeddingIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL)
	h := NewReportHandler(services.NewReportService(
		repository.NewGuestRepo(db), repository.NewTableRepo(db), repository.NewWeddingRepo(db)))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	s.Mux.Handle("GET /api/weddings/{wid}/reports/angpao", http.HandlerFunc(h.ExportAngpao))

	req := httptest.NewRequest(http.MethodGet, "/api/weddings/"+utils.EncodeUUID(uuid.New())+"/reports/angpao", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body.String())
	}
}

func TestReport_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL)
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Wedding"}).Error)
	h := NewReportHandler(services.NewReportService(
		repository.NewGuestRepo(db), repository.NewTableRepo(db), repository.NewWeddingRepo(db)))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	s.Mux.Handle("GET /api/weddings/{wid}/reports/angpao", http.HandlerFunc(h.ExportAngpao))

	req := httptest.NewRequest(http.MethodGet, "/api/weddings/"+utils.EncodeUUID(wid)+"/reports/angpao", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}
