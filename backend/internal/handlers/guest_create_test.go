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

// TestGuestCreate_KeepsAngbaoAndGift covers the create payload fields the
// optimistic client already echoes.
func TestGuestCreate_KeepsAngbaoAndGift(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)

	wid := uuid.New()
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(`{"name":"Alice","pax":1,"angbaoAmt":200,"giftItem":"vase"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body %s", rec.Code, rec.Body.String())
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.Where("wedding_id = ?", wid).First(&got).Error)
	if got.AngbaoAmt == nil || *got.AngbaoAmt != 200 || got.GiftItem == nil || *got.GiftItem != "vase" {
		t.Fatalf("stored angbao/gift = %v/%v, want 200/vase", got.AngbaoAmt, got.GiftItem)
	}
	if rec := post(`{"name":"Bob","pax":1,"angbaoAmt":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative angbao status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
}

// TestGuestImport_KeepsAngbaoAndGift covers the bulk create payload fields the
// optimistic client echoes.
func TestGuestImport_KeepsAngbaoAndGift(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/import", NewGuestHandler(svc).BulkImport)

	wid := uuid.New()
	body := `{"guests":[{"name":"Alice","pax":1,"angbaoAmt":200,"giftItem":"vase"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body %s", rec.Code, rec.Body.String())
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.Where("wedding_id = ?", wid).First(&got).Error)
	if got.AngbaoAmt == nil || *got.AngbaoAmt != 200 || got.GiftItem == nil || *got.GiftItem != "vase" {
		t.Fatalf("stored angbao/gift = %v/%v, want 200/vase", got.AngbaoAmt, got.GiftItem)
	}
}

// A seat number without a table is rejected at create, like bulk import does.
func TestGuestCreate_SeatWithoutTableIs400(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)

	wid := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests",
		strings.NewReader(`{"name":"Alice","pax":1,"seatNum":3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
}

// A create retried with its client ID returns the stored guest with 200 and no
// second row; the same ID under another wedding is 409 without its data.
func TestGuestCreate_ClientIDIsIdempotent(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)

	post := func(wid uuid.UUID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+utils.EncodeUUID(wid)+"/guests", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	wid, gid := uuid.New(), uuid.New()
	first := `{"id":"` + utils.EncodeUUID(gid) + `","name":"Alice","pax":1}`
	if rec := post(wid, first); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201, body %s", rec.Code, rec.Body.String())
	}
	rec := post(wid, `{"id":"`+gid.String()+`","name":"Alice retried","pax":1}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Alice"`) {
		t.Fatalf("retry = %d %s, want 200 with the stored guest", rec.Code, rec.Body.String())
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Count(&count).Error)
	if count != 1 {
		t.Fatalf("guest rows = %d, want 1", count)
	}
	rec = post(uuid.New(), `{"id":"`+gid.String()+`","name":"Mallory","pax":1}`)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "Alice") {
		t.Fatalf("other wedding = %d %s, want 409 without the guest", rec.Code, rec.Body.String())
	}
	if rec := post(wid, `{"id":"not-an-id!","name":"Bob","pax":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", rec.Code)
	}
}
