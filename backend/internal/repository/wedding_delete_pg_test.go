package repository

import (
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestWeddingDelete_NoOrphanGuestAfterConcurrentCreate proves the cascade takes
// the wedding row lock first: a guest created while the delete waits on that
// lock must be swept by the cascade, not left behind the wedding row's removal.
func TestWeddingDelete_NoOrphanGuestAfterConcurrentCreate(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testutil.PgDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("postgres open: %v", err)
	}
	// The test drops tables; never let it point at a real database.
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, banquet_tables, wedding_events, hall_elements, user_weddings CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}, &models.HallElement{}, &models.UserWedding{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewWeddingRepo(db)
	ctx := t.Context()
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Delete Race"}).Error)
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: uuid.New(), WeddingID: wid, Name: "T1", Capacity: 4}).Error)
	testutil.NoErr(t, db.Create(&models.HallElement{ID: uuid.New(), WeddingID: wid, Name: "Bar", Type: "bar"}).Error)
	testutil.NoErr(t, db.Create(&models.UserWedding{ID: uuid.New(), UserID: uuid.New(), WeddingID: wid}).Error)

	// Hold the wedding row lock, so the cascade cannot start.
	lockTx := db.Begin()
	testutil.NoErr(t, lockTx.Error)
	t.Cleanup(func() { lockTx.Rollback() })
	var locked models.WeddingEvent
	testutil.NoErr(t, lockTx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").Where("id = ?", wid).First(&locked).Error)

	done := make(chan error, 1)
	go func() { done <- repo.Delete(ctx, wid) }()

	select {
	case err := <-done:
		t.Fatalf("delete finished before the wedding lock was released: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	// A guest created under the held lock must not survive the cascade.
	orphan := models.GuestRecord{
		ID: uuid.New(), WeddingID: wid, Name: "Orphan", Pax: 1, RSVP: "pending",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	testutil.NoErr(t, lockTx.Create(&orphan).Error)
	testutil.NoErr(t, lockTx.Commit().Error)

	select {
	case err := <-done:
		testutil.NoErr(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the wedding delete")
	}

	var guests int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&guests).Error)
	if guests != 0 {
		t.Fatalf("guest rows = %d, want 0: the cascade left an orphan", guests)
	}
	for name, model := range map[string]any{
		"tables":        &models.BanquetTable{},
		"hall elements": &models.HallElement{},
		"user grants":   &models.UserWedding{},
	} {
		var count int64
		testutil.NoErr(t, db.Model(model).Where("wedding_id = ?", wid).Count(&count).Error)
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0: the cascade missed them", name, count)
		}
	}
	var weddings int64
	testutil.NoErr(t, db.Model(&models.WeddingEvent{}).Where("id = ?", wid).Count(&weddings).Error)
	if weddings != 0 {
		t.Fatalf("wedding rows = %d, want 0", weddings)
	}
}
