package repository

import (
	"context"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The layout save reconciles inside its transaction: a stored element missing
// from the payload is deleted, a new one is created and an existing one updated
// in one write.
func TestLayoutRepo_SaveLayoutReplacesElementSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE wedding_events (
		id TEXT PRIMARY KEY, name TEXT, hall_width INTEGER, hall_height INTEGER, updated_at DATETIME)`).Error)
	testutil.NoErr(t, db.Exec(`CREATE TABLE hall_elements (
		id TEXT PRIMARY KEY, wedding_id TEXT, type TEXT, x REAL, y REAL, degree REAL,
		width REAL, height REAL, name TEXT, color TEXT, text_color TEXT, stroke_color TEXT,
		opacity REAL, z_index INTEGER, created_at DATETIME, updated_at DATETIME)`).Error)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)
	ctx := context.Background()
	wid := uuid.New()
	testutil.NoErr(t, db.Exec(`INSERT INTO wedding_events (id, name, hall_width, hall_height) VALUES (?, 'W', 860, 1000)`, wid.String()).Error)

	keep := models.HallElement{ID: uuid.New(), WeddingID: wid, Type: "stage", Width: 10, Height: 10}
	drop := models.HallElement{ID: uuid.New(), WeddingID: wid, Type: "tv", Width: 10, Height: 10}
	testutil.NoErr(t, db.Create(&keep).Error)
	testutil.NoErr(t, db.Create(&drop).Error)

	updated := keep
	updated.X = 42
	incoming := []models.HallElement{
		updated,
		{WeddingID: wid, Type: "box", X: 10, Y: 10, Width: 20, Height: 20},
	}
	testutil.NoErr(t, NewLayoutRepo(db).SaveLayout(ctx, wid, 1000, 1200, nil, incoming))

	var stored []models.HallElement
	testutil.NoErr(t, db.Where("wedding_id = ?", wid).Order("type").Find(&stored).Error)
	if len(stored) != 2 {
		t.Fatalf("elements = %+v, want stage and box", stored)
	}
	var stage *models.HallElement
	for i := range stored {
		if stored[i].ID == keep.ID {
			stage = &stored[i]
		}
	}
	if stage == nil || stage.X != 42 {
		t.Fatalf("stage = %+v, want X updated to 42", stage)
	}

	var w models.WeddingEvent
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Select("hall_width", "hall_height").Where("id = ?", wid).Scan(&w).Error)
	if w.HallWidth != 1000 || w.HallHeight != 1200 {
		t.Fatalf("hall = %dx%d, want 1000x1200", w.HallWidth, w.HallHeight)
	}
}
