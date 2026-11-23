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

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// A DB outage during a lookup is 500, not "not found".
func TestWeddingList_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL)
	wid := uuid.New()
	h := NewWeddingHandler(services.NewWeddingService(repository.NewWeddingRepo(db)), repository.NewAdminRepo(db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/weddings", h.List)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	req := httptest.NewRequest(http.MethodGet, "/api/weddings", nil)
	ctx := context.WithValue(req.Context(), middleware.RoleKey, "user")
	ctx = context.WithValue(ctx, middleware.WeddingIDKey, &wid)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

func TestAdminDelete_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, adminUsersDDL)
	h := NewAdminHandler(repository.NewAdminRepo(db), nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Delete(s, "/admin/users/{id}", h.Delete)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/"+uuid.New().String(), strings.NewReader("null"))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.RoleKey, "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

func TestAdminResetPassword_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, adminUsersDDL)
	h := NewAdminHandler(repository.NewAdminRepo(db), nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/admin/users/{id}/reset-password", h.ResetPassword)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+uuid.New().String()+"/reset-password",
		strings.NewReader(`{"password":"NewPassw0rd!"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.RoleKey, "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

func TestGuestGet_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL)
	h := NewGuestHandler(services.NewGuestService(repository.NewGuestRepo(db), nil, db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/weddings/{wid}/guests/{id}", h.Get)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	req := httptest.NewRequest(http.MethodGet,
		"/api/weddings/"+uuid.New().String()+"/guests/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

func TestTableUpdate_QueryFailureIs500(t *testing.T) {
	db := newSQLite(t, banquetTablesDDL)
	h := NewTableHandler(services.NewTableService(repository.NewTableRepo(db), nil, db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/tables/{id}", h.Update)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	body := `{"name":"A","capacity":2,"x":0,"y":0,"degree":0,"isVip":false}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/weddings/"+uuid.New().String()+"/tables/"+uuid.New().String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

// A table for a nonexistent wedding is 404 and inserts nothing (no FK).
func TestTableCreate_MissingWeddingIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL, banquetTablesDDL)
	h := NewTableHandler(services.NewTableService(repository.NewTableRepo(db), nil, db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/tables", h.Create)

	wid := uuid.New()
	body := `{"name":"A","capacity":2,"x":0,"y":0,"degree":0,"isVip":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+wid.String()+"/tables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Table("banquet_tables").Where("wedding_id = ?", wid).Count(&count).Error)
	if count != 0 {
		t.Fatalf("created %d orphan tables, want 0", count)
	}
}

// Saving a layout that names a missing table is 404, not 500.
func TestLayoutSave_MissingTableIs404(t *testing.T) {
	db := newSQLite(t, weddingEventsDDL, banquetTablesDDL, guestRecordsDDL,
		`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT, type TEXT, x REAL, y REAL,
			degree REAL, width REAL, height REAL, name TEXT, color TEXT, text_color TEXT,
			stroke_color TEXT, opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`)
	h := NewLayoutHandler(
		services.NewLayoutService(repository.NewLayoutRepo(db)),
		services.NewTableService(repository.NewTableRepo(db), nil, db),
		services.NewWeddingService(repository.NewWeddingRepo(db)),
	)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/layout", h.Save)

	body := `{"hallWidth":860,"hallHeight":1000,"tables":[{"id":"` + uuid.New().String() +
		`","x":1,"y":1,"degree":0,"capacity":2,"isVip":false}],"elements":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+uuid.New().String()+"/layout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body.String())
	}
}
