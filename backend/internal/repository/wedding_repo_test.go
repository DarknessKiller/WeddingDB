package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A wedding save must not re-insert a row another request deleted: Save would,
// and the wedding would silently come back.
func TestWeddingRepo_UpdateDetectsMissingRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT, date DATETIME, venue_name TEXT, venue_address TEXT,
		kiosk_description TEXT, kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER, kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER, kiosk_idle_reset_seconds INTEGER,
		hall_width INTEGER, hall_height INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	// ":memory:" gives every new connection its own empty database, so keep one.
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	repo := NewWeddingRepo(db)
	ctx := context.Background()

	w := &models.WeddingEvent{ID: uuid.New(), Name: "Old", VenueName: "Hall A", Date: time.Now().UTC().Truncate(time.Second)}
	testutil.NoErr(t, db.Create(w).Error)
	w.Name = "New"
	testutil.NoErr(t, repo.Update(ctx, w))

	var got models.WeddingEvent
	testutil.NoErr(t, db.First(&got, "id = ?", w.ID).Error)
	if got.Name != "New" || got.VenueName != "Hall A" {
		t.Fatalf("wedding = %q/%q, want New/Hall A", got.Name, got.VenueName)
	}

	testutil.NoErr(t, db.Where("id = ?", w.ID).Delete(&models.WeddingEvent{}).Error)
	if err := repo.Update(ctx, w); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update deleted wedding err = %v, want gorm.ErrRecordNotFound", err)
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("deleted wedding re-inserted: %d rows", count)
	}
}
