package handlers

import (
	"net/http"
	"strings"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A create with an over-cap name must be a client error, not a 500 or a stored
// row that blows up the kiosk payload.
func TestCreateGuest_RejectsOversizedName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT, name TEXT, pax INTEGER)`).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := services.NewGuestService(repository.NewGuestRepo(db), nil, nil, db)

	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Post(s, "/api/weddings/{wid}/guests", NewGuestHandler(svc).Create)

	body := `{"name":"` + strings.Repeat("a", services.MaxGuestNameLen+1) + `","pax":1}`
	rec := serveGuest(t, s, http.MethodPost, "/api/weddings/"+uuid.New().String()+"/guests", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized name status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
