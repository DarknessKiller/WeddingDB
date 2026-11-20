package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

func serveGuest(t *testing.T, s *fuego.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	return rec
}

// Pax above the party-size cap must fail before the service is reached; the
// handlers are wired with a nil service so a pass-through would panic.
func TestCreateGuest_RejectsPaxOverLimit(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(nil).Create)
	wid := uuid.New().String()
	body := `{"name":"Big","pax":` + strconv.Itoa(services.MaxGuestPartySize+1) + `}`

	rec := serveGuest(t, s, http.MethodPost, "/api/weddings/"+wid+"/guests", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create over-limit pax status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateGuest_RejectsPaxOverLimit(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Put(s, "/api/weddings/{wid}/guests/{id}", NewGuestHandler(nil).Update)
	wid := uuid.New().String()
	gid := uuid.New().String()
	body := `{"name":"Big","pax":` + strconv.Itoa(services.MaxGuestPartySize+1) + `}`

	rec := serveGuest(t, s, http.MethodPut, "/api/weddings/"+wid+"/guests/"+gid, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update over-limit pax status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBulkImport_RejectsPaxOverLimit(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/bulk", NewGuestHandler(nil).BulkImport)
	wid := uuid.New().String()
	body := `{"guests":[{"name":"Big","pax":` + strconv.Itoa(services.MaxGuestPartySize+1) + `}]}`

	rec := serveGuest(t, s, http.MethodPost, "/api/weddings/"+wid+"/guests/bulk", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bulk over-limit pax status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
