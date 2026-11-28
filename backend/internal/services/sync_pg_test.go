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

// TestSyncCheckOut_PostgresLosesToCheckInCommittedWhileWaiting covers invariant
// 3 under the real row lock: a check-out that waited for the guest row must lose
// to a check-in with a later client clock that committed while it waited.
// Requires TEST_DATABASE_URL pointing at a *_test database.
func TestSyncCheckOut_PostgresLosesToCheckInCommittedWhileWaiting(t *testing.T) {
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
	svc := NewGuestService(repository.NewGuestRepo(db), nil, db)
	wid, gid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Checkout Race"}).Error)
	base := time.Now().Add(-10 * time.Minute).UTC()
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Arriving", Pax: 1, RSVP: "pending",
		CreatedAt: base, UpdatedAt: base,
	}).Error)

	checkOutAt := base.Add(time.Minute)
	checkInAt := base.Add(2 * time.Minute)

	// Hold the guest row lock and commit a check-in inside it: the queued
	// check-out waits on that row until the check-in is committed.
	lockTx := db.Begin()
	testutil.NoErr(t, lockTx.Error)
	var locked models.GuestRecord
	testutil.NoErr(t, lockTx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", gid).First(&locked).Error)
	testutil.NoErr(t, lockTx.Model(&models.GuestRecord{}).Where("id = ?", gid).Updates(map[string]any{
		"checked_in_at": checkInAt,
		"rsvp":          "confirmed",
		"field_times":   models.FieldTimes{groupCheckedInAt: checkInAt, groupRSVP: checkInAt},
	}).Error)

	type syncOutcome struct {
		results []SyncResult
		err     error
	}
	done := make(chan syncOutcome, 1)
	go func() {
		res, err := svc.Sync(ctx, wid, []SyncMutation{{
			Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: checkOutAt,
		}})
		done <- syncOutcome{results: res, err: err}
	}()
	// Let the check-out reach the row lock before the check-in commits. The
	// assertions hold either way: the lock and the client clocks both decide it.
	time.Sleep(200 * time.Millisecond)
	testutil.NoErr(t, lockTx.Commit().Error)

	var out syncOutcome
	select {
	case out = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the check-out to finish")
	}
	testutil.NoErr(t, out.err)
	if out.results[0].Status != "skipped" {
		t.Fatalf("check-out older than the committed check-in = %+v, want skipped", out.results[0])
	}
	if out.results[0].ServerRecord == nil || out.results[0].ServerRecord.CheckedInAt == nil {
		t.Fatalf("skipped result must carry the checked-in server row, got %+v", out.results[0].ServerRecord)
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.CheckedInAt == nil || g.RSVP != "confirmed" {
		t.Fatalf("guest = checkedInAt %v rsvp %q, want the committed check-in kept", g.CheckedInAt, g.RSVP)
	}
}
