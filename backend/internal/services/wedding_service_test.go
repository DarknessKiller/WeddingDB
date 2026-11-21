package services

import (
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWeddingCreate_SeedsDefaultHall(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME,
		venue_name TEXT, venue_address TEXT, kiosk_description TEXT, kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER, kiosk_background_size TEXT, kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT, show_seat_numbers INTEGER,
		kiosk_idle_reset_seconds INTEGER, hall_width INTEGER, hall_height INTEGER, layout_version INTEGER,
		created_at DATETIME, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE hall_elements (
		id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))), wedding_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
		x REAL, y REAL, degree REAL, width REAL, height REAL, color TEXT, text_color TEXT,
		stroke_color TEXT, opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	svc := NewWeddingService(repository.NewWeddingRepo(db))

	w := &models.WeddingEvent{ID: uuid.New(), Name: "W"}
	testutil.NoErr(t, svc.Create(t.Context(), w))

	var n int64
	testutil.NoErr(t, db.Model(&models.HallElement{}).Where("wedding_id = ?", w.ID).Count(&n).Error)
	if want := len(models.DefaultElements(w.ID)); int(n) != want {
		t.Fatalf("hall elements = %d, want %d", n, want)
	}
}
