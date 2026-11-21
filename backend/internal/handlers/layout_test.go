package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// Text over a column limit must be a 400 before the service runs; the
// handlers are wired with nil services so a pass-through would panic.
func TestLayoutAndTable_RejectOverlongText(t *testing.T) {
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Patch(s, "/api/weddings/{wid}/layout", NewLayoutHandler(nil, nil, nil).Save)
	fuego.Post(s, "/api/weddings/{wid}/tables", NewTableHandler(nil).Create)
	wid := uuid.New().String()
	element := func(field, value string) string {
		return `{"hallWidth":10,"hallHeight":10,"elements":[{"type":"stage","x":1,"y":1,"width":1,"height":1,"` +
			field + `":"` + value + `"}]}`
	}
	cases := []struct{ name, method, path, body string }{
		{"element name", http.MethodPatch, "/api/weddings/" + wid + "/layout", element("name", strings.Repeat("n", 256))},
		{"element color", http.MethodPatch, "/api/weddings/" + wid + "/layout", element("color", strings.Repeat("c", 21))},
		{"element text color", http.MethodPatch, "/api/weddings/" + wid + "/layout", element("textColor", strings.Repeat("c", 21))},
		{"element stroke color", http.MethodPatch, "/api/weddings/" + wid + "/layout", element("strokeColor", strings.Repeat("c", 21))},
		{"table name", http.MethodPost, "/api/weddings/" + wid + "/tables", `{"name":"` + strings.Repeat("t", 101) + `","capacity":10}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveGuest(t, s, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}
