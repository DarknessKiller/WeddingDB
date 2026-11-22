package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// TestGuestUpdate_SparsePatch exercises the real Fuego route: a PUT body carries
// only the keys it changes, absent keys keep their stored value, and an explicit
// null clears the seat group.
func TestGuestUpdate_SparsePatch(t *testing.T) {
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
	} {
		testutil.NoErr(t, db.Exec(stmt).Error)
	}

	wid := uuid.New()
	tid := uuid.New()
	seat := 2
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 8}).Error)
	guest := &models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Alice", Email: "alice@example.com",
		Pax: 1, RSVP: "pending", Notes: "keep", Dietary: models.StringSlice{"vegan"},
		TableID: &tid, SeatNum: &seat,
	}
	testutil.NoErr(t, db.Create(guest).Error)

	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/guests/{id}", NewGuestHandler(svc).Update)

	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut,
			"/api/weddings/"+utils.EncodeUUID(wid)+"/guests/"+utils.EncodeUUID(guest.ID),
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	decode := func(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
		return got
	}

	rec := put(`{"pax":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("sparse update status = %d, body %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got["pax"] != float64(3) || got["notes"] != "keep" || got["name"] != "Alice" || got["tableId"] == nil {
		t.Fatalf("absent fields must be preserved, got %v", got)
	}

	rec = put(`{"tableId":null,"seatNum":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("null seat status = %d, body %s", rec.Code, rec.Body.String())
	}
	got = decode(t, rec)
	if got["tableId"] != nil || got["seatNum"] != nil {
		t.Fatalf("null must clear seat, got table=%v seat=%v", got["tableId"], got["seatNum"])
	}
	if got["notes"] != "keep" || got["pax"] != float64(3) {
		t.Fatalf("clearing the seat must not touch other fields, got %v", got)
	}

	rec = put(`{"pax":0}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid pax status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}

	var stored models.GuestRecord
	testutil.NoErr(t, db.First(&stored, "id = ?", guest.ID).Error)
	if stored.Pax != 3 || stored.Notes != "keep" || stored.TableID != nil {
		t.Fatalf("stored row = pax %d notes %q table %v", stored.Pax, stored.Notes, stored.TableID)
	}
}
