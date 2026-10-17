package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"weddingdb/internal/middleware"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
	"weddingdb/internal/utils"
)

// TestUpdateKioskSettings_PostgresRoundTrip proves a percentage position is
// accepted verbatim, invalid values are rejected by field name, and a partial
// payload does not clear untouched fields. Requires a real Postgres:
//
//	TEST_DATABASE_URL=postgresql://weddingdb@127.0.0.1:25432/weddingdb_test?sslmode=disable \
//	  go test ./internal/handlers -run TestUpdateKioskSettings_PostgresRoundTrip -v
func TestUpdateKioskSettings_PostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("postgres open: %v", err)
	}
	// The test drops the table; never let it point at a real database.
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() { db.Exec("DROP TABLE IF EXISTS wedding_events CASCADE") }
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	wid := uuid.New()
	if err := db.Create(&models.WeddingEvent{
		ID: wid, Name: "Round Trip", VenueName: "Old Hall", VenueAddress: "Old Address",
		KioskBackgroundPosX: "center", KioskBackgroundPosY: "center",
	}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}

	admin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.RoleKey, "admin")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	s := fuego.NewServer(fuego.WithoutStartupMessages())
	fuego.Use(s, admin)
	h := NewWeddingHandler(services.NewWeddingService(repository.NewWeddingRepo(db)), repository.NewAdminRepo(db))
	fuego.Put(s, "/api/weddings/{id}/kiosk", h.UpdateKioskSettings)
	fuego.Get(s, "/api/weddings/{id}", h.Get)

	id := utils.EncodeUUID(wid)
	do := func(method, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, "/api/weddings/"+id, nil)
		} else {
			req = httptest.NewRequest(method, "/api/weddings/"+id+"/kiosk", strings.NewReader(body))
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Mux.ServeHTTP(rec, req)
		return rec
	}
	decode := func(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response %q: %v", rec.Body.String(), err)
		}
		return got
	}

	t.Run("percent round-trips through update and get", func(t *testing.T) {
		rec := do(http.MethodPut, `{"kioskBackgroundPosX":"37%"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, body %s", rec.Code, rec.Body.String())
		}
		got := decode(t, rec)
		if got["kioskBackgroundPosX"] != "37%" {
			t.Errorf("update returned kioskBackgroundPosX = %v, want 37%%", got["kioskBackgroundPosX"])
		}
		if got["venueName"] != "Old Hall" || got["venueAddress"] != "Old Address" {
			t.Errorf("partial payload cleared omitted fields: %v", got)
		}

		rec = do(http.MethodGet, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, body %s", rec.Code, rec.Body.String())
		}
		got = decode(t, rec)
		if got["kioskBackgroundPosX"] != "37%" {
			t.Errorf("persisted kioskBackgroundPosX = %v, want 37%%", got["kioskBackgroundPosX"])
		}
		if got["kioskBackgroundPosY"] != "center" {
			t.Errorf("omitted kioskBackgroundPosY = %v, want center", got["kioskBackgroundPosY"])
		}
		if got["venueName"] != "Old Hall" {
			t.Errorf("persisted venueName = %v, want Old Hall", got["venueName"])
		}
	})

	t.Run("invalid value is rejected with the field name", func(t *testing.T) {
		rec := do(http.MethodPut, `{"kioskBackgroundPosY":"999%"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "kioskBackgroundPosY") {
			t.Errorf("error body does not name kioskBackgroundPosY: %s", rec.Body.String())
		}
	})
}
