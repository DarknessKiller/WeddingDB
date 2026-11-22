package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestGuestCreate_NameBounds exercises the real Fuego create route: a name over
// the column cap is a 400, while a name exactly at the cap is accepted.
func TestGuestCreate_NameBounds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1,
		table_id TEXT, seat_num INTEGER, rsvp TEXT DEFAULT 'no_response',
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER,
		angbao_amt INTEGER, gift_item TEXT, field_times TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error)
	wid := uuid.New()
	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)
	post := func(name string) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"name": name, "pax": 1})
		testutil.NoErr(t, err)
		req := httptest.NewRequest(http.MethodPost,
			"/api/weddings/"+utils.EncodeUUID(wid)+"/guests", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(strings.Repeat("x", services.MaxNameLen+1)); rec.Code != http.StatusBadRequest {
		t.Fatalf("256-char name status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := post(strings.Repeat("x", services.MaxNameLen)); rec.Code == http.StatusBadRequest {
		t.Fatalf("255-char name rejected: %d, body %s", rec.Code, rec.Body.String())
	}
}
