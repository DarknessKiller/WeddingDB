package handlers

import (
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
)

const guestRecordsDDL = `CREATE TABLE guest_records (
	id TEXT PRIMARY KEY DEFAULT '00000000-0000-0000-0000-000000000000',
	wedding_id TEXT NOT NULL, name TEXT NOT NULL,
	name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1,
	table_id TEXT, seat_num INTEGER, rsvp TEXT DEFAULT 'no_response',
	checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER,
	angbao_amt INTEGER, gift_item TEXT, field_times TEXT,
	created_at DATETIME, updated_at DATETIME)`

const banquetTablesDDL = `CREATE TABLE banquet_tables (
	id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
	capacity INTEGER, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
	degree REAL NOT NULL DEFAULT 0, is_vip INTEGER,
	created_at DATETIME, updated_at DATETIME)`

// A seat failure must roll back the guest insert; the old two-transaction path
// could leave a guest with no seat.
func TestGuestCreate_SeatFailureLeavesNoRow(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)

	wid := uuid.New()
	body := `{"name":"Alice","pax":1,"tableId":"` + utils.EncodeUUID(uuid.New()) + `","seatNum":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&count).Error)
	if count != 0 {
		t.Fatalf("seat failure left %d guest rows, want 0", count)
	}
}
