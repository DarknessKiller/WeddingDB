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
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

const adminUsersDDL = `CREATE TABLE admin_users (
	id TEXT PRIMARY KEY, email TEXT, password TEXT, name TEXT, role TEXT,
	force_password_change INTEGER, created_at DATETIME, updated_at DATETIME)`

const userWeddingsDDL = `CREATE TABLE user_weddings (
	id TEXT PRIMARY KEY, user_id TEXT, wedding_id TEXT, created_at DATETIME)`

func withAdmin(req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.RoleKey, "admin"))
}

func TestAdminCreate_InvalidWeddingIDIs400(t *testing.T) {
	db := newSQLite(t, adminUsersDDL, userWeddingsDDL)
	h := NewAdminHandler(repository.NewAdminRepo(db), nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/users", h.Create)

	body := `{"email":"u@x.test","name":"User","password":"valid-pass1!","weddings":["not-an-id"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAdmin(req)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "weddings") {
		t.Fatalf("status = %d, body %s, want 400 naming weddings", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.AdminUser{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("invalid wedding ID created %d accounts, want 0", count)
	}
}

func TestAssignWeddings_InvalidWeddingIDIs400(t *testing.T) {
	db := newSQLite(t, adminUsersDDL, userWeddingsDDL)
	uid := uuid.New()
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: uid, Email: "u@x.test", Role: "user"}).Error)
	h := NewAdminHandler(repository.NewAdminRepo(db), nil)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/users/{id}/weddings", h.AssignWeddings)

	req := httptest.NewRequest(http.MethodPut, "/api/users/"+utils.EncodeUUID(uid)+"/weddings",
		strings.NewReader(`{"weddings":["not-an-id"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = withAdmin(req)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "weddings") {
		t.Fatalf("status = %d, body %s, want 400 naming weddings", rec.Code, rec.Body.String())
	}
}

func TestBulkImport_OutageIs500(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/import", NewGuestHandler(svc).BulkImport)

	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())

	body := `{"guests":[{"name":"Alice","pax":1}]}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/weddings/"+utils.EncodeUUID(uuid.New())+"/guests/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body %s", rec.Code, rec.Body.String())
	}
}

func TestBulkImport_InvalidTableIDIs400(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/import", NewGuestHandler(svc).BulkImport)

	body := `{"guests":[{"name":"Alice","pax":1,"tableId":"not-an-id"}]}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/weddings/"+utils.EncodeUUID(uuid.New())+"/guests/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "guests[0].tableId") {
		t.Fatalf("status = %d, body %s, want 400 naming guests[0].tableId", rec.Code, rec.Body.String())
	}
}
