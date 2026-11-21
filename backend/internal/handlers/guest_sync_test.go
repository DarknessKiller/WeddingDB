package handlers

import (
	"encoding/json"
	"net/http"
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

// One mutation the server cannot decode must fail alone: the rest of the
// offline queue still applies instead of the whole batch being refused.
func TestSync_MalformedMutationFailsAlone(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.AutoMigrate(&models.GuestTombstone{}))
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL, name_pinyin TEXT,
		email TEXT, pax INTEGER NOT NULL DEFAULT 1, table_id TEXT, seat_num INTEGER, rsvp TEXT,
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER, angbao_amt INTEGER,
		gift_item TEXT, created_at DATETIME, updated_at DATETIME, field_updated_at TEXT)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL, capacity INTEGER,
		x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, degree REAL NOT NULL DEFAULT 0,
		is_vip INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)

	svc := services.NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/sync", NewGuestHandler(svc).Sync)

	at := time.Now().UTC().Format(time.RFC3339Nano)
	kept := uuid.New()
	body := `{"mutations":[` +
		`{"mutationId":"bad","op":"create","guestId":"` + uuid.New().String() + `","clientUpdatedAt":"` + at + `","payload":{"name":"Ann","pax":"two"}},` +
		`{"mutationId":"ok","op":"create","guestId":"` + kept.String() + `","clientUpdatedAt":"` + at + `","payload":{"name":"Bo","pax":1}}]}`
	rec := serveGuest(t, s, http.MethodPost, "/api/weddings/"+uuid.New().String()+"/guests/sync", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Results []struct {
			Status    string `json:"status"`
			Reason    string `json:"reason"`
			Retryable bool   `json:"retryable"`
		} `json:"results"`
	}
	testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &got))
	if len(got.Results) != 2 {
		t.Fatalf("results = %d, want 2: %s", len(got.Results), rec.Body.String())
	}
	if got.Results[0].Status != "failed" || got.Results[0].Retryable {
		t.Fatalf("malformed mutation = %s retryable=%v (%s), want non-retryable failed",
			got.Results[0].Status, got.Results[0].Retryable, got.Results[0].Reason)
	}
	if got.Results[1].Status != "applied" {
		t.Fatalf("valid mutation = %s (%s), want applied", got.Results[1].Status, got.Results[1].Reason)
	}
	var n int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", kept).Count(&n).Error)
	if n != 1 {
		t.Fatalf("valid mutation created %d rows, want 1", n)
	}
}
