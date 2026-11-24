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
	testutil.NoErr(t, repo.Update(ctx, w.ID, map[string]any{"name": w.Name}))

	var got models.WeddingEvent
	testutil.NoErr(t, db.First(&got, "id = ?", w.ID).Error)
	if got.Name != "New" || got.VenueName != "Hall A" {
		t.Fatalf("wedding = %q/%q, want New/Hall A", got.Name, got.VenueName)
	}

	testutil.NoErr(t, db.Where("id = ?", w.ID).Delete(&models.WeddingEvent{}).Error)
	if err := repo.Update(ctx, w.ID, map[string]any{"name": w.Name}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update deleted wedding err = %v, want gorm.ErrRecordNotFound", err)
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("deleted wedding re-inserted: %d rows", count)
	}
}

// newWeddingRepoDB opens a single-connection sqlite database with the wedding
// row and the tables the delete cascade touches.
func newWeddingRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	// ":memory:" gives every new connection its own empty database, so keep one.
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT, date DATETIME, venue_name TEXT, venue_address TEXT,
		kiosk_description TEXT, kiosk_logo_url TEXT, kiosk_background_url TEXT,
		kiosk_background_blur INTEGER, kiosk_background_size TEXT,
		kiosk_background_pos_x TEXT, kiosk_background_pos_y TEXT,
		kiosk_logo_size TEXT, kiosk_logo_pos_x TEXT, kiosk_logo_pos_y TEXT,
		show_seat_numbers INTEGER, kiosk_idle_reset_seconds INTEGER,
		hall_width INTEGER, hall_height INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	for _, ddl := range []string{
		`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE banquet_tables (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE hall_elements (id TEXT PRIMARY KEY, wedding_id TEXT)`,
		`CREATE TABLE user_weddings (id TEXT PRIMARY KEY, wedding_id TEXT)`,
	} {
		testutil.NoErr(t, db.Exec(ddl).Error)
	}
	return db
}

// A wedding save writes only the columns the caller carried: two partial saves
// of different kiosk fields must keep both.
func TestWeddingRepo_UpdateWritesCarriedColumnsOnly(t *testing.T) {
	db := newWeddingRepoDB(t)
	repo := NewWeddingRepo(db)
	ctx := context.Background()
	w := &models.WeddingEvent{
		ID: uuid.New(), Name: "Kiosk", Date: time.Now().UTC().Truncate(time.Second),
		KioskLogoUrl: "/old-logo.png", KioskBackgroundUrl: "/old-bg.png",
	}
	testutil.NoErr(t, db.Create(w).Error)

	testutil.NoErr(t, repo.Update(ctx, w.ID, map[string]any{"kiosk_logo_url": "/new-logo.png"}))
	testutil.NoErr(t, repo.Update(ctx, w.ID, map[string]any{"kiosk_background_url": "/new-bg.png"}))

	var got models.WeddingEvent
	testutil.NoErr(t, db.First(&got, "id = ?", w.ID).Error)
	if got.KioskLogoUrl != "/new-logo.png" {
		t.Fatalf("logo = %q, want /new-logo.png; the second save reverted it", got.KioskLogoUrl)
	}
	if got.KioskBackgroundUrl != "/new-bg.png" {
		t.Fatalf("background = %q, want /new-bg.png", got.KioskBackgroundUrl)
	}
}

// Deleting a wedding that does not exist must report not-found instead of a
// success for a write that matched no row.
func TestWeddingRepo_DeleteMissingRowIsNotFound(t *testing.T) {
	db := newWeddingRepoDB(t)
	if err := NewWeddingRepo(db).Delete(context.Background(), uuid.New()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("delete missing wedding err = %v, want gorm.ErrRecordNotFound", err)
	}
}
