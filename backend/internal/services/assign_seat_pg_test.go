package services

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestAssignSeat_PostgresConcurrent races 20 guests for one seat; exactly one
// may win. Requires TEST_DATABASE_URL pointing at a *_test database.
func TestAssignSeat_PostgresConcurrent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("postgres open: %v", err)
	}
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q", dbName)
	}
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, banquet_tables, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	ctx := context.Background()
	wid, tid := uuid.New(), uuid.New()
	if err := db.Create(&models.WeddingEvent{ID: wid, Name: "Seat"}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}
	if err := db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error; err != nil {
		t.Fatalf("seed table: %v", err)
	}
	const n = 20
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
		if err := db.Create(&models.GuestRecord{ID: ids[i], WeddingID: wid, Name: "G", Pax: 1}).Error; err != nil {
			t.Fatalf("seed guest: %v", err)
		}
	}
	for round := range 50 {
		if err := db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).
			Updates(map[string]any{"table_id": nil, "seat_num": nil}).Error; err != nil {
			t.Fatalf("reset seats: %v", err)
		}
		var wins atomic.Int32
		var wg sync.WaitGroup
		for _, gid := range ids {
			wg.Go(func() {
				if svc.AssignSeat(ctx, gid, wid, tid, 1) == nil {
					wins.Add(1)
				}
			})
		}
		wg.Wait()
		if got := wins.Load(); got != 1 {
			t.Fatalf("round %d: winners = %d, want 1", round, got)
		}
	}
}
