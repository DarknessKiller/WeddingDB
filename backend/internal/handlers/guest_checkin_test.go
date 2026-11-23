package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestCheckIn_ReadBackFailureStillAnswersCommittedRow proves a check-in that
// committed answers 200 with the committed row: the response no longer depends
// on a read back after the write, which used to turn a landed check-in into a
// 500 and a retry into a bogus "already checked in" conflict.
func TestCheckIn_ReadBackFailureStillAnswersCommittedRow(t *testing.T) {
	db := newSQLite(t, `CREATE TABLE guest_records (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		name_pinyin TEXT, email TEXT, pax INTEGER NOT NULL DEFAULT 1,
		table_id TEXT, seat_num INTEGER, rsvp TEXT DEFAULT 'no_response',
		checked_in_at DATETIME, notes TEXT, dietary TEXT, is_vip INTEGER,
		angbao_amt INTEGER, gift_item TEXT, field_times TEXT,
		created_at DATETIME, updated_at DATETIME)`)

	wid := uuid.New()
	guest := &models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Arriving", Pax: 2, RSVP: "pending", Notes: "keep",
	}
	testutil.NoErr(t, db.Create(guest).Error)

	// Fail any guest read after the one the check-in itself makes. The write
	// has committed by then, so the handler must not read the row back.
	var failReadBack atomic.Bool
	var reads atomic.Int32
	testutil.NoErr(t, db.Callback().Query().Before("gorm:query").Register("fail_read_back", func(tx *gorm.DB) {
		if !failReadBack.Load() {
			return
		}
		if _, ok := tx.Statement.Dest.(*models.GuestRecord); ok && reads.Add(1) > 1 {
			tx.AddError(errors.New("read back failed")) //nolint:errcheck // AddError echoes the error just added
		}
	}))

	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/checkin", NewGuestHandler(svc).CheckIn)

	req := httptest.NewRequest(http.MethodPost,
		"/api/weddings/"+utils.EncodeUUID(wid)+"/guests/"+utils.EncodeUUID(guest.ID)+"/checkin",
		strings.NewReader(`{"notes":"came late"}`))
	req.Header.Set("Content-Type", "application/json")
	failReadBack.Store(true)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	failReadBack.Store(false)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a committed check-in, body %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	testutil.NoErr(t, json.Unmarshal(rec.Body.Bytes(), &got))
	if got["checkedInAt"] == nil {
		t.Fatalf("response must carry the committed check-in, got %v", got)
	}
	if got["notes"] != "keep\ncame late" || got["rsvp"] != "confirmed" || got["pax"] != float64(2) {
		t.Fatalf("response = %v, want the merged row", got)
	}

	var stored models.GuestRecord
	testutil.NoErr(t, db.First(&stored, "id = ?", guest.ID).Error)
	if stored.CheckedInAt == nil || stored.RSVP != "confirmed" {
		t.Fatalf("stored row = checkedInAt %v rsvp %q, want the check-in committed", stored.CheckedInAt, stored.RSVP)
	}
}
