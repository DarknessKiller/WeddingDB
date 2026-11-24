package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

func bulkImportPost(t *testing.T, s *fuego.Server, wid uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/weddings/"+wid.String()+"/guests/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	return rec
}

// Client seat/pax errors in a bulk import are 400, not 500: the fix must route
// BulkCreate sentinels through the same mapping Create uses.
func TestBulkImport_ClientSeatErrorsAre400(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/import", NewGuestHandler(svc).BulkImport)
	wid := uuid.New()

	tableID := uuid.New()
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "A", Capacity: 2}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Occupant", Pax: 1, TableID: &tableID, SeatNum: new(1),
	}).Error)

	t.Run("seat overlap", func(t *testing.T) {
		body := `{"guests":[{"name":"Late","pax":1,"tableId":"` + tableID.String() + `","seatNum":1}]}`
		rec := bulkImportPost(t, s, wid, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("pax over limit", func(t *testing.T) {
		body := `{"guests":[{"name":"Big","pax":` + strconv.Itoa(services.MaxGuestPartySize+1) + `}]}`
		rec := bulkImportPost(t, s, wid, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("seat without table", func(t *testing.T) {
		body := `{"guests":[{"name":"Seatless","pax":1,"seatNum":1}]}`
		rec := bulkImportPost(t, s, wid, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "seatNum") {
			t.Fatalf("body does not name seatNum: %s", rec.Body.String())
		}
	})
}
