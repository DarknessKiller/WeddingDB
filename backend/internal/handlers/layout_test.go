package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/testutil"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The public layout route has no scope middleware, so an unknown wedding must
// be a 404 from the handler, not an unhandled 500.
func TestLayoutGet_UnknownWeddingIsNotFound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT, name TEXT)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT, z_index INTEGER)`).Error)
	h := NewLayoutHandler(
		services.NewLayoutService(repository.NewLayoutRepo(db)),
		services.NewTableService(repository.NewTableRepo(db), repository.NewGuestRepo(db), nil, db),
		services.NewWeddingService(repository.NewWeddingRepo(db)),
	)
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Get(s, "/api/public/weddings/{wid}/layout", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/public/weddings/"+uuid.New().String()+"/layout", nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
