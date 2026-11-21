package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGuestTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL, name_pinyin TEXT,
		email TEXT, pax INTEGER NOT NULL DEFAULT 1, table_id TEXT, seat_num INTEGER,
		rsvp TEXT DEFAULT 'no_response', checked_in_at DATETIME, notes TEXT, dietary TEXT,
		is_vip INTEGER, angbao_amt INTEGER, gift_item TEXT, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL, capacity INTEGER,
		x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, degree REAL NOT NULL DEFAULT 0,
		is_vip INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME, hall_width INTEGER DEFAULT 860,
		hall_height INTEGER DEFAULT 1000, layout_version INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE hall_elements (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
		x REAL, y REAL, degree REAL, width REAL, height REAL, color TEXT, text_color TEXT,
		stroke_color TEXT, opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	return db
}

func newGuestTestServer(db *gorm.DB) *fuego.Server {
	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	h := NewGuestHandler(svc)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", h.Create)
	fuego.Put(s, "/api/weddings/{wid}/guests/{id}", h.Update)
	fuego.Delete(s, "/api/weddings/{wid}/guests/{id}/seat", h.UnassignSeat)
	return s
}

func serveJSON(s *fuego.Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	return rec
}

func TestGuestUpdate_PreservesCheckIn(t *testing.T) {
	db := newGuestTestDB(t)
	wid, gid := uuid.New(), uuid.New()
	checkedIn := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Alice", Pax: 1, CheckedInAt: &checkedIn}).Error)

	// A stale edit form knows nothing about the check-in.
	rec := serveJSON(newGuestTestServer(db), http.MethodPut, "/api/weddings/"+wid.String()+"/guests/"+gid.String(), `{"name":"Alicia","pax":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.Name != "Alicia" || got.Pax != 2 {
		t.Fatalf("edit not applied: %+v", got)
	}
	if got.CheckedInAt == nil {
		t.Fatal("edit cleared checked_in_at")
	}
	if !strings.Contains(rec.Body.String(), `"checkedInAt":"`) {
		t.Fatalf("response lost checkedInAt: %s", rec.Body)
	}
}

func TestGuestUnassignSeat(t *testing.T) {
	db := newGuestTestDB(t)
	wid, gid, tid := uuid.New(), uuid.New(), uuid.New()
	seat := 3
	checkedIn := time.Now().UTC()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Bob", Pax: 1, TableID: &tid, SeatNum: &seat, CheckedInAt: &checkedIn, Notes: "vip"}).Error)
	s := newGuestTestServer(db)

	tests := []struct {
		name string
		wid  uuid.UUID
		want int
	}{
		{"other wedding", uuid.New(), http.StatusNotFound},
		{"own wedding", wid, http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveJSON(s, http.MethodDelete, "/api/weddings/"+tt.wid.String()+"/guests/"+gid.String()+"/seat", "")
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.TableID != nil || got.SeatNum != nil {
		t.Fatalf("seat not cleared: table=%v seat=%v", got.TableID, got.SeatNum)
	}
	if got.CheckedInAt == nil || got.Notes != "vip" {
		t.Fatalf("unassign touched other columns: %+v", got)
	}
}

func TestGuestCheckOut_OnlyClearsCheckIn(t *testing.T) {
	db := newGuestTestDB(t)
	wid, gid := uuid.New(), uuid.New()
	checkedIn := time.Now().UTC()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Cara", Pax: 1, CheckedInAt: &checkedIn, Notes: "n"}).Error)
	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)

	testutil.NoErr(t, svc.CheckOut(t.Context(), gid, wid))
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt != nil || got.Notes != "n" {
		t.Fatalf("checkout: %+v", got)
	}
}
