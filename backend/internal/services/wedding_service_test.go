package services

import (
	"context"
	"testing"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A grant failure must roll back the wedding insert: the old two-transaction
// path left an ownerless wedding that a retry duplicated.
func TestWeddingCreateWithOwner_GrantFailureLeavesNoWedding(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, date DATETIME,
		venue_name TEXT, venue_address TEXT, kiosk_description TEXT,
		kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER DEFAULT 0, kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER DEFAULT 1, kiosk_idle_reset_seconds INTEGER DEFAULT 0,
		hall_width INTEGER DEFAULT 860, hall_height INTEGER DEFAULT 1000,
		created_at DATETIME, updated_at DATETIME)`).Error) // no user_weddings table

	svc := NewWeddingService(repository.NewWeddingRepo(db))
	w := &models.WeddingEvent{ID: uuid.New(), Name: "Rollback"}
	if err := svc.CreateWithOwner(context.Background(), w, uuid.New()); err == nil {
		t.Fatal("grant failure returned nil")
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Where("id = ?", w.ID).Count(&count).Error)
	if count != 0 {
		t.Fatalf("grant failure left %d wedding rows, want 0", count)
	}
}
