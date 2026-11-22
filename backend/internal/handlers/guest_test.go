package handlers

import (
	"encoding/json"
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
		is_vip INTEGER, angbao_amt INTEGER, gift_item TEXT, created_at DATETIME, updated_at DATETIME, field_updated_at TEXT)`).Error)
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
	fuego.Delete(s, "/api/weddings/{wid}/guests/{id}", h.Delete)
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

func TestGuestCreate_SeatInSameTransaction(t *testing.T) {
	db := newGuestTestDB(t)
	wid, tid := uuid.New(), uuid.New()
	seat := 3
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 4}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Seated", Pax: 1, TableID: &tid, SeatNum: &seat}).Error)
	s := newGuestTestServer(db)
	path := "/api/weddings/" + wid.String() + "/guests"

	tests := []struct {
		name, body string
		want       int
	}{
		{"seat taken", `{"name":"A","pax":1,"tableId":"` + tid.String() + `","seatNum":3}`, http.StatusBadRequest},
		{"over capacity", `{"name":"B","pax":2,"tableId":"` + tid.String() + `","seatNum":4}`, http.StatusBadRequest},
		{"bad table id", `{"name":"C","pax":1,"tableId":"nope"}`, http.StatusBadRequest},
		{"free seat", `{"name":"D","pax":1,"tableId":"` + tid.String() + `","seatNum":1}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveJSON(s, http.MethodPost, path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
	var names []string
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Order("name").Pluck("name", &names).Error)
	if strings.Join(names, ",") != "D,Seated" {
		t.Fatalf("guests = %v, want only D and Seated", names)
	}
}

func TestGuestCreate_ClientID(t *testing.T) {
	db := newGuestTestDB(t)
	wid, otherWid := uuid.New(), uuid.New()
	gid, foreign := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: foreign, WeddingID: otherWid, Name: "Other", Pax: 1}).Error)
	s := newGuestTestServer(db)
	path := "/api/weddings/" + wid.String() + "/guests"

	tests := []struct {
		name, body string
		want       int
	}{
		{"first create", `{"id":"` + gid.String() + `","name":"Ann","pax":1}`, http.StatusCreated},
		{"retry returns stored guest", `{"id":"` + gid.String() + `","name":"Ann changed","pax":1}`, http.StatusCreated},
		{"other wedding's id", `{"id":"` + foreign.String() + `","name":"X","pax":1}`, http.StatusConflict},
		{"invalid id", `{"id":"nope","name":"Y","pax":1}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveJSON(s, http.MethodPost, path, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
	var got []models.GuestRecord
	testutil.NoErr(t, db.Where("wedding_id = ?", wid).Find(&got).Error)
	if len(got) != 1 || got[0].ID != gid || got[0].Name != "Ann" {
		t.Fatalf("guests = %+v, want one Ann with the client id", got)
	}
	var other models.GuestRecord
	testutil.NoErr(t, db.First(&other, "id = ?", foreign).Error)
	if other.WeddingID != otherWid || other.Name != "Other" {
		t.Fatalf("foreign guest changed: %+v", other)
	}
}

func TestSync_MalformedMutationFailsAlone(t *testing.T) {
	db := newGuestTestDB(t)
	testutil.NoErr(t, db.Exec(`CREATE TABLE applied_mutations (wedding_id TEXT, mutation_id TEXT, created_at DATETIME, PRIMARY KEY (wedding_id, mutation_id))`).Error)
	wid, gid := uuid.New(), uuid.New()
	base := time.Now().Add(-time.Hour).UTC()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Ann", Pax: 1, CreatedAt: base, UpdatedAt: base}).Error)
	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/sync", NewGuestHandler(svc).Sync)

	body := `{"mutations":[
		{"guestId":"bad","op":"update","clientUpdatedAt":"not a time"},
		{"mutationId":"m2","guestId":"` + gid.String() + `","op":"update","clientUpdatedAt":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) + `","payload":{"name":"Bea"}}
	]}`
	rec := serveJSON(s, http.MethodPost, "/api/weddings/"+wid.String()+"/guests/sync", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		Results []struct {
			GuestID   string
			Status    string
			Retryable bool
		}
	}
	testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &got))
	if len(got.Results) != 2 {
		t.Fatalf("results = %+v", got.Results)
	}
	if r := got.Results[0]; r.Status != "failed" || r.Retryable || r.GuestID != "bad" {
		t.Fatalf("malformed item: %+v, want failed, not retryable", r)
	}
	if got.Results[1].Status != "applied" {
		t.Fatalf("valid item: %+v, want applied", got.Results[1])
	}
	if !strings.Contains(rec.Body.String(), `"retryable":false`) {
		t.Fatalf("response lacks retryable field: %s", rec.Body)
	}
}

func TestGuestDelete_MissingIs404(t *testing.T) {
	db := newGuestTestDB(t)
	wid, gid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Ann", Pax: 1}).Error)
	s := newGuestTestServer(db)
	path := "/api/weddings/" + wid.String() + "/guests/"

	tests := []struct {
		name string
		id   uuid.UUID
		want int
	}{
		{"missing guest", uuid.New(), http.StatusNotFound},
		{"existing guest", gid, http.StatusNoContent},
		{"already deleted", gid, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := serveJSON(s, http.MethodDelete, path+tt.id.String(), ""); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// A form that loaded a row before another device saved it must be rejected,
// not allowed to overwrite the newer values.
func TestGuestUpdate_StaleVersionConflicts(t *testing.T) {
	db := newGuestTestDB(t)
	wid, gid := uuid.New(), uuid.New()
	loaded := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Alice", Pax: 1, UpdatedAt: loaded}).Error)
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).
		Updates(map[string]any{"name": "Bob", "notes": "saved elsewhere", "updated_at": loaded.Add(time.Minute)}).Error)

	s := newGuestTestServer(db)
	stale := `{"name":"Alicia","pax":1,"updatedAt":"` + loaded.Format(time.RFC3339Nano) + `"}`
	if rec := serveJSON(s, http.MethodPut, "/api/weddings/"+wid.String()+"/guests/"+gid.String(), stale); rec.Code != http.StatusConflict {
		t.Fatalf("stale edit status = %d, want 409; body %s", rec.Code, rec.Body)
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.Name != "Bob" || got.Notes != "saved elsewhere" {
		t.Fatalf("stale edit overwrote newer row: %+v", got)
	}

	fresh := `{"name":"Alicia","pax":1,"updatedAt":"` + loaded.Add(time.Minute).Format(time.RFC3339Nano) + `"}`
	if rec := serveJSON(s, http.MethodPut, "/api/weddings/"+wid.String()+"/guests/"+gid.String(), fresh); rec.Code != http.StatusOK {
		t.Fatalf("fresh edit status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}
