package handlers

import (
	"net/http"
	"testing"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// A bulk row with an undecodable tableId must reject the import, not silently
// drop the table assignment and create an unseated guest.
func TestBulkImport_RejectsInvalidTableID(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/bulk", NewGuestHandler(nil).BulkImport)
	wid := uuid.New().String()
	body := `{"guests":[{"name":"Alice","tableId":"not-a-table-id"}]}`

	rec := serveGuest(t, s, http.MethodPost, "/api/weddings/"+wid+"/guests/bulk", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bulk invalid tableId status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
