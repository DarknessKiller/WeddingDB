package bootstrap

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openMigrateTestDB returns a connection to TEST_DATABASE_URL with the app
// tables dropped. It refuses anything whose name does not end in _test.
func openMigrateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, wedding_events, hall_elements, banquet_tables, admin_users, refresh_tokens, user_weddings CASCADE")
	}
	drop()
	t.Cleanup(drop)
	return db
}

func TestMigrate_PostgresPinyinBackfill(t *testing.T) {
	db := openMigrateTestDB(t)
	ctx := context.Background()
	testutil.NoErr(t, Migrate(ctx, db))

	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Backfill"}).Error)
	guests := []models.GuestRecord{
		{ID: uuid.New(), WeddingID: wid, Name: "张三", Pax: 1},
		{ID: uuid.New(), WeddingID: wid, Name: "李四", Pax: 1},
	}
	testutil.NoErr(t, db.Create(&guests).Error)

	testutil.NoErr(t, Migrate(ctx, db))
	for _, g := range guests {
		var got string
		testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", g.ID).Pluck("name_pinyin", &got).Error)
		if got != models.GenerateNamePinyin(g.Name) {
			t.Errorf("guest %s pinyin = %q, want %q", g.Name, got, models.GenerateNamePinyin(g.Name))
		}
	}
}

// Two concurrent migrate jobs both succeed, and the default hall elements are
// seeded exactly once (the advisory lock serializes the check-and-create).
func TestMigrate_PostgresConcurrentRuns(t *testing.T) {
	db := openMigrateTestDB(t)
	ctx := context.Background()
	testutil.NoErr(t, Migrate(ctx, db))

	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Concurrent"}).Error)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = Migrate(ctx, db) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent migrate %d failed: %v", i, err)
		}
	}

	var count int64
	testutil.NoErr(t, db.Model(&models.HallElement{}).Where("wedding_id = ?", wid).Count(&count).Error)
	if want := int64(len(models.DefaultElements(wid))); count != want {
		t.Fatalf("seeded %d hall elements, want %d", count, want)
	}
}
