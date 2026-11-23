package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

// TestGuestSeat_MissingGuestIs404 covers the handler mapping for a seat write
// on a guest that does not exist. The wedding lock is a no-op on sqlite, so the
// guest lookup is what misses here.
func TestGuestSeat_MissingGuestIs404(t *testing.T) {
	db := newSQLite(t, guestRecordsDDL, banquetTablesDDL)
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, db)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests/{id}/seat", NewGuestHandler(svc).AssignSeat)
	fuego.Delete(s, "/api/weddings/{wid}/guests/{id}/seat", NewGuestHandler(svc).UnassignSeat)

	wid := utils.EncodeUUID(uuid.New())
	gid := utils.EncodeUUID(uuid.New())
	post := httptest.NewRequest(http.MethodPost, "/api/weddings/"+wid+"/guests/"+gid+"/seat", strings.NewReader(`{"tableId":"`+utils.EncodeUUID(uuid.New())+`","seatNum":1}`))
	post.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	s.Mux.ServeHTTP(postRec, post)
	if postRec.Code != http.StatusNotFound {
		t.Fatalf("assign seat status = %d, want 404, body %s", postRec.Code, postRec.Body.String())
	}

	delRec := httptest.NewRecorder()
	s.Mux.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/api/weddings/"+wid+"/guests/"+gid+"/seat", nil))
	if delRec.Code != http.StatusNotFound {
		t.Fatalf("unassign seat status = %d, want 404, body %s", delRec.Code, delRec.Body.String())
	}
}
