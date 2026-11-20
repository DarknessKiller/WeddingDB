package services

import (
	"context"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newWeddingServiceEnv(t *testing.T) (*WeddingService, *gorm.DB, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	// Raw DDL: the model's uuid default is Postgres-only.
	if err := db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		date DATETIME,
		venue_name TEXT,
		venue_address TEXT,
		kiosk_description TEXT,
		kiosk_logo_url TEXT,
		kiosk_background_url TEXT,
		kiosk_background_blur INTEGER,
		kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT,
		kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT,
		kiosk_logo_pos_x TEXT,
		kiosk_logo_pos_y TEXT,
		show_seat_numbers NUMERIC DEFAULT false,
		kiosk_idle_reset_seconds INTEGER NOT NULL DEFAULT 0,
		hall_width INTEGER NOT NULL DEFAULT 860,
		hall_height INTEGER NOT NULL DEFAULT 1000,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewWeddingService(repository.NewWeddingRepo(db))
	wid := uuid.New()
	if err := db.Create(&models.WeddingEvent{ID: wid, Name: "Old Name", VenueName: "Old Hall", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}
	return svc, db, wid
}

// Two editors loading the same wedding and saving different fields must not
// overwrite each other with their stale copies of the row.
func TestWeddingUpdate_DoesNotClobberConcurrentFields(t *testing.T) {
	svc, _, wid := newWeddingServiceEnv(t)
	ctx := context.Background()

	basic, err := svc.Get(ctx, wid)
	if err != nil {
		t.Fatalf("load for basic edit: %v", err)
	}
	kiosk, err := svc.Get(ctx, wid)
	if err != nil {
		t.Fatalf("load for kiosk edit: %v", err)
	}

	basic.Name = "New Name"
	basic.Date = time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	if err := svc.UpdateBasic(ctx, basic); err != nil {
		t.Fatalf("basic update: %v", err)
	}

	kiosk.VenueName = "New Hall"
	if err := svc.UpdateKiosk(ctx, kiosk, false); err != nil {
		t.Fatalf("kiosk update: %v", err)
	}

	got, err := svc.Get(ctx, wid)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Name != "New Name" {
		t.Fatalf("name = %q, kiosk save clobbered it with the stale value", got.Name)
	}
	if got.Date.Year() != 2026 || got.Date.Month() != time.June {
		t.Fatalf("date = %v, kiosk save clobbered it with the stale value", got.Date)
	}
	if got.VenueName != "New Hall" {
		t.Fatalf("venueName = %q, want New Hall", got.VenueName)
	}

	// A kiosk payload that does carry a name still writes it.
	kiosk.Name = "Kiosk Renamed"
	if err := svc.UpdateKiosk(ctx, kiosk, true); err != nil {
		t.Fatalf("kiosk rename: %v", err)
	}
	got, err = svc.Get(ctx, wid)
	if err != nil {
		t.Fatalf("reload after rename: %v", err)
	}
	if got.Name != "Kiosk Renamed" {
		t.Fatalf("name = %q, kiosk payload with a name did not write it", got.Name)
	}
}
