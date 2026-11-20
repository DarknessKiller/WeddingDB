package handlers

import (
	"net/http"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A DELETE for a guest that is gone must answer 404, not a success status
// that hides the write from the client.
func TestDeleteGuest_MissingReturns404(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT, name TEXT)`).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, nil, db)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Delete(s, "/api/weddings/{wid}/guests/{id}", NewGuestHandler(svc).Delete)

	rec := serveGuest(t, s, http.MethodDelete,
		"/api/weddings/"+uuid.New().String()+"/guests/"+uuid.New().String(), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing guest status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
