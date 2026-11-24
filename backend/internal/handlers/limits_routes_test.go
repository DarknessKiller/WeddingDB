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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These tests exercise the real Fuego routes for caps enforced only in the
// handler, where no service-level rejection exists to fall back on.

func TestTableCreate_NameBounds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	for _, stmt := range []string{
		`CREATE TABLE guest_records (
			id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
			name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1,
			table_id TEXT, seat_num INTEGER, rsvp TEXT DEFAULT 'no_response',
			checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER,
			angbao_amt INTEGER, gift_item TEXT, field_times TEXT,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE banquet_tables (
			id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
			capacity INTEGER, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
			degree REAL NOT NULL DEFAULT 0, is_vip INTEGER,
			created_at DATETIME, updated_at DATETIME)`,
		weddingEventsDDL,
	} {
		testutil.NoErr(t, db.Exec(stmt).Error)
	}
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Wedding"}).Error)

	svc := services.NewTableService(repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/tables", NewTableHandler(svc).Create)

	post := func(name string) *httptest.ResponseRecorder {
		body := `{"name":"` + name + `","capacity":8}`
		req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/tables", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(strings.Repeat("a", services.MaxTableNameLen+1)); rec.Code != http.StatusBadRequest {
		t.Fatalf("101-char table name status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := post(strings.Repeat("a", services.MaxTableNameLen)); rec.Code != http.StatusCreated {
		t.Fatalf("100-char table name status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestKioskSettings_VenueNameBounds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME,
		venue_name TEXT, venue_address TEXT, kiosk_description TEXT,
		kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER DEFAULT 0, kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER DEFAULT 1, kiosk_idle_reset_seconds INTEGER DEFAULT 0,
		hall_width INTEGER DEFAULT 860, hall_height INTEGER DEFAULT 1000,
		created_at DATETIME, updated_at DATETIME)`).Error)

	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Wedding"}).Error)

	ws := services.NewWeddingService(repository.NewWeddingRepo(db))
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{id}/kiosk", NewWeddingHandler(ws, repository.NewAdminRepo(db)).UpdateKioskSettings)

	put := func(venueName string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut,
			"/api/weddings/"+utils.EncodeUUID(wid)+"/kiosk",
			strings.NewReader(`{"venueName":"`+venueName+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), middleware.RoleKey, "admin"))
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := put(strings.Repeat("a", services.MaxVenueNameLen+1)); rec.Code != http.StatusBadRequest {
		t.Fatalf("501-char venue name status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := put(strings.Repeat("a", services.MaxVenueNameLen)); rec.Code != http.StatusOK {
		t.Fatalf("500-char venue name status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestLayoutSave_ElementCountCap(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/layout", NewLayoutHandler(nil, nil, nil).Save)

	put := func(elements int) *httptest.ResponseRecorder {
		items := make([]string, 0, elements)
		for range elements {
			items = append(items, `{"type":"box","x":10,"y":10,"width":10,"height":10}`)
		}
		body := `{"hallWidth":1000,"hallHeight":1000,"tables":[],"elements":[` + strings.Join(items, ",") + `]}`
		req := httptest.NewRequest(http.MethodPut, "/api/weddings/"+utils.EncodeUUID(uuid.New())+"/layout", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := put(services.MaxLayoutElements + 1); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "Too many elements") {
		t.Fatalf("%d elements status = %d, body %s", services.MaxLayoutElements+1, rec.Code, rec.Body.String())
	}
}

func TestLayoutSave_ElementNameBounds(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/layout", NewLayoutHandler(nil, nil, nil).Save)

	put := func(name string) *httptest.ResponseRecorder {
		body := `{"hallWidth":1000,"hallHeight":1000,"tables":[],"elements":[` +
			`{"type":"stage","x":10,"y":10,"width":10,"height":10,"name":"` + name + `"}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/weddings/"+utils.EncodeUUID(uuid.New())+"/layout", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := put(strings.Repeat("a", services.MaxNameLen+1)); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "Element name") {
		t.Fatalf("256-char element name status = %d, body %s", rec.Code, rec.Body.String())
	}
}
