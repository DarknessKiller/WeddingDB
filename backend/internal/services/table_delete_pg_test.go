package services

import (
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestTableDelete_DoesNotRevertConcurrentCheckIn proves the delete snapshots
// the affected guests under the row lock: a check-in that commits while the
// delete waits for the lock must survive the unassign. Requires
// TEST_DATABASE_URL pointing at a *_test database.
func TestTableDelete_DoesNotRevertConcurrentCheckIn(t *testing.T) {
	dsn := testutil.PgDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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
		db.Exec("DROP TABLE IF EXISTS guest_records, guest_tombstones, banquet_tables, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}, &models.GuestTombstone{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	svc := NewTableService(repository.NewTableRepo(db), nil, db)
	wid, tid, gid := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Delete Race"}).Error)
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 8}).Error)
	seat := 3
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Seated", Pax: 2, Notes: "keep",
		TableID: &tid, SeatNum: &seat,
	}).Error)

	// Hold the guest row lock and commit a check-in inside it. The delete must
	// wait for this lock, so its snapshot has to include the check-in.
	lockTx := db.Begin()
	testutil.NoErr(t, lockTx.Error)
	var locked models.GuestRecord
	testutil.NoErr(t, lockTx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", gid).First(&locked).Error)
	checkedInAt := time.Now()
	notes := appendNote(locked.Notes, "came late")
	testutil.NoErr(t, lockTx.Model(&models.GuestRecord{}).Where("id = ?", gid).Updates(map[string]any{
		"checked_in_at": checkedInAt,
		"rsvp":          "confirmed",
		"notes":         notes,
		"field_times": models.FieldTimes{
			groupCheckedInAt: checkedInAt, groupRSVP: checkedInAt, groupNotes: checkedInAt,
		},
	}).Error)

	done := make(chan error, 1)
	go func() { done <- svc.Delete(ctx, tid, wid) }()

	// While the check-in is uncommitted the delete is blocked on the row lock,
	// so neither the table nor the seated guest may change.
	select {
	case err := <-done:
		t.Fatalf("delete finished before the check-in committed: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	var during models.BanquetTable
	testutil.NoErr(t, db.Where("id = ?", tid).First(&during).Error)
	var seated models.GuestRecord
	testutil.NoErr(t, db.Where("id = ?", gid).First(&seated).Error)
	if seated.TableID == nil {
		t.Fatal("guest was unseated before the delete could lock the row")
	}

	testutil.NoErr(t, lockTx.Commit().Error)
	select {
	case err := <-done:
		testutil.NoErr(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the delete to finish")
	}

	g, err := repository.NewGuestRepo(db).FindByID(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt == nil || g.RSVP != "confirmed" {
		t.Fatalf("guest = checkedInAt %v rsvp %q, want the committed check-in kept", g.CheckedInAt, g.RSVP)
	}
	if g.Notes != notes {
		t.Fatalf("notes = %q, want %q", g.Notes, notes)
	}
	if g.TableID != nil || g.SeatNum != nil {
		t.Fatalf("guest still seated at the deleted table: table %v seat %v", g.TableID, g.SeatNum)
	}
}
