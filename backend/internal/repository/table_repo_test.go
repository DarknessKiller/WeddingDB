package repository

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTableRepo_UpdateAndDeleteDetectMissingRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE banquet_tables (
		id TEXT PRIMARY KEY, wedding_id TEXT NOT NULL, name TEXT NOT NULL,
		capacity INTEGER, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
		degree REAL NOT NULL DEFAULT 0, is_vip INTEGER,
		created_at DATETIME, updated_at DATETIME)`).Error)
	sqlDB, err := db.DB()
	testutil.NoErr(t, err)
	sqlDB.SetMaxOpenConns(1)

	ctx := context.Background()
	repo := NewTableRepo(db)
	tbl := &models.BanquetTable{ID: uuid.New(), WeddingID: uuid.New(), Name: "T1", Capacity: 8}
	testutil.NoErr(t, repo.Create(ctx, tbl))

	tbl.Name = "T2"
	testutil.NoErr(t, repo.Update(ctx, tbl))

	testutil.NoErr(t, db.Where("id = ?", tbl.ID).Delete(&models.BanquetTable{}).Error)
	if err := repo.Update(ctx, tbl); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("update deleted table err = %v, want gorm.ErrRecordNotFound", err)
	}
	if err := repo.Delete(ctx, tbl.ID, tbl.WeddingID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("delete missing table err = %v, want gorm.ErrRecordNotFound", err)
	}
	var count int64
	testutil.NoErr(t, db.Model(&models.BanquetTable{}).Count(&count).Error)
	if count != 0 {
		t.Fatalf("deleted table re-inserted: %d rows", count)
	}
}
