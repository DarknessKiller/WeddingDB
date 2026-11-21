package handlers

import (
	"net/http"
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

// newGuestCreateServer serves guest create on sqlite with one table of the
// given capacity.
func newGuestCreateServer(t *testing.T, capacity int) (*fuego.Server, *gorm.DB, string, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1, table_id TEXT, seat_num INTEGER, rsvp TEXT,
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER, angbao_amt INTEGER, gift_item TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		capacity INTEGER, x REAL, y REAL, degree REAL, is_vip INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	wid, tid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: capacity}).Error)

	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)
	return s, db, "/api/weddings/" + wid.String() + "/guests", utils.EncodeUUID(tid)
}

// A table with room but seat 1 taken accepts a guest sent without a seat
// number, the same as import and sync: no seat is invented.
func TestCreateGuest_TableWithoutSeatNum(t *testing.T) {
	s, db, path, table := newGuestCreateServer(t, 10)
	if rec := serveGuest(t, s, http.MethodPost, path, `{"name":"Ann","pax":1,"tableId":"`+table+`","seatNum":1}`); rec.Code != http.StatusCreated {
		t.Fatalf("seat 1: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := serveGuest(t, s, http.MethodPost, path, `{"name":"Bo","pax":1,"tableId":"`+table+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("table only: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var g models.GuestRecord
	testutil.NoErr(t, db.Where("name = ?", "Bo").First(&g).Error)
	if g.TableID == nil || g.SeatNum != nil {
		t.Fatalf("table = %v seat = %v, want table set and no seat", g.TableID, g.SeatNum)
	}
}

// Creating a guest with a seat is one write: a refused seat leaves no guest
// behind, and the angbao and gift in the body are kept.
func TestCreateGuest_SeatAndGiftInOneWrite(t *testing.T) {
	s, db, path, table := newGuestCreateServer(t, 2)

	if rec := serveGuest(t, s, http.MethodPost, path, `{"name":"Big","pax":3,"tableId":"`+table+`","seatNum":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("party of 3 at a table of 2: status = %d, want 400", rec.Code)
	}
	var n int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Count(&n).Error)
	if n != 0 {
		t.Fatalf("refused create left %d guest rows", n)
	}

	if rec := serveGuest(t, s, http.MethodPost, path, `{"name":"Ann","pax":1,"tableId":"`+table+`","seatNum":2,"angbaoAmt":88,"giftItem":"Tea set"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var g models.GuestRecord
	testutil.NoErr(t, db.First(&g).Error)
	if g.AngbaoAmt == nil || *g.AngbaoAmt != 88 || g.GiftItem == nil || *g.GiftItem != "Tea set" {
		t.Fatalf("angbao=%v gift=%v, want 88 and Tea set", g.AngbaoAmt, g.GiftItem)
	}
	if g.SeatNum == nil || *g.SeatNum != 2 {
		t.Fatalf("seat = %v, want 2", g.SeatNum)
	}
}

// Seat refusals answer 400 with their reason on every guest route, and a
// missing guest answers 404; before, PUT sent a full table to the 500 handler.
func TestGuestSeatErrors_MapTo400And404(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1, table_id TEXT, seat_num INTEGER, rsvp TEXT,
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER, angbao_amt INTEGER, gift_item TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		capacity INTEGER, x REAL, y REAL, degree REAL, is_vip INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	wid, tid, gid := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 2}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Big", Pax: 3}).Error)

	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/guests/{id}", NewGuestHandler(svc).Update)
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/seat", NewGuestHandler(svc).AssignSeat)
	guests := "/api/weddings/" + utils.EncodeUUID(wid) + "/guests/"
	table := utils.EncodeUUID(tid)

	if rec := serveGuest(t, s, http.MethodPut, guests+utils.EncodeUUID(gid), `{"name":"Big","pax":3,"tableId":"`+table+`","seatNum":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT party of 3 to a table of 2: status = %d, want 400", rec.Code)
	}
	if rec := serveGuest(t, s, http.MethodPost, guests+utils.EncodeUUID(gid)+"/seat", `{"tableId":"`+table+`","seatNum":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("assign party of 3 to a table of 2: status = %d, want 400", rec.Code)
	}
	if rec := serveGuest(t, s, http.MethodPost, guests+utils.EncodeUUID(uuid.New())+"/seat", `{"tableId":"`+table+`","seatNum":1}`); rec.Code != http.StatusNotFound {
		t.Fatalf("assign a missing guest: status = %d, want 404", rec.Code)
	}
}

// A name of only spaces is no name: create and update must refuse it.
func TestGuestWrites_RejectWhitespaceName(t *testing.T) {
	s, _, path, _ := newGuestCreateServer(t, 10)
	if rec := serveGuest(t, s, http.MethodPost, path, `{"name":"   ","pax":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
