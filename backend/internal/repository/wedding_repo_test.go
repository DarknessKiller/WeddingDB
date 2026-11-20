package repository

import (
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A new wedding gets the default hall layout at creation, not on the next boot.
func TestWeddingRepo_CreateSeedsDefaultHall(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT, date DATETIME,
		venue_name TEXT, venue_address TEXT, kiosk_description TEXT, kiosk_logo_url TEXT,
		kiosk_background_url TEXT, kiosk_background_blur INTEGER, kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT, kiosk_logo_size TEXT,
		kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT, show_seat_numbers BOOLEAN,
		kiosk_idle_reset_seconds INTEGER, hall_width INTEGER, hall_height INTEGER,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT, type TEXT,
		x REAL, y REAL, degree REAL, width REAL, height REAL, name TEXT, color TEXT, text_color TEXT,
		stroke_color TEXT, opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)

	w := &models.WeddingEvent{ID: uuid.New(), Name: "New", Date: time.Now()}
	testutil.NoErr(t, NewWeddingRepo(db).Create(t.Context(), w))

	els, err := NewLayoutRepo(db).ElementsByWedding(t.Context(), w.ID)
	testutil.NoErr(t, err)
	if want := len(models.DefaultElements(w.ID)); len(els) != want {
		t.Fatalf("new wedding has %d hall elements, want %d defaults", len(els), want)
	}
}
