package services

import (
	"errors"
	"strings"
	"sync"
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

// openGuestPG opens TEST_DATABASE_URL with fresh guest tables and a service.
func openGuestPG(t *testing.T) (*GuestService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(postgres.Open(pgDSN(t)), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q", dbName)
	}
	drop := func() { db.Exec("DROP TABLE IF EXISTS guest_records, banquet_tables, wedding_events CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}))
	return NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db), db
}

// Two phones syncing the same offline check-in at once: each note must land,
// so neither write was built on a copy that missed the other.
func TestSyncCheckIn_PostgresConcurrentKeepsBothNotes(t *testing.T) {
	svc, db := openGuestPG(t)
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	for round := range 30 {
		gid := uuid.New()
		testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "G", Pax: 1}).Error)
		// One arrival time for both, so neither counts as older than the other.
		at := time.Now()
		var wg sync.WaitGroup
		for _, note := range []string{"desk A", "desk B"} {
			wg.Go(func() {
				res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
					Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: at, Payload: &SyncPayload{Notes: syncStr(note)},
				}})
				if err != nil || res[0].Status != "applied" {
					t.Errorf("sync %s: %v %+v", note, err, res)
				}
			})
		}
		wg.Wait()
		var got models.GuestRecord
		testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
		if !strings.Contains(got.Notes, "desk A") || !strings.Contains(got.Notes, "desk B") {
			t.Fatalf("round %d: notes = %q, want both desks' notes", round, got.Notes)
		}
	}
}

// A seat assignment waiting on the wedding lock must use the party size the
// lock holder committed, not the one it read before waiting.
func TestAssignSeat_PostgresUsesPaxCommittedWhileWaiting(t *testing.T) {
	svc, db := openGuestPG(t)
	wid, tid, gid := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tid, WeddingID: wid, Name: "T1", Capacity: 10}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "G", Pax: 2}).Error)

	// Hold the wedding lock the way a guest edit does, while raising pax to 4.
	tx := db.Begin()
	testutil.NoErr(t, tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", wid).First(&models.WeddingEvent{}).Error)
	testutil.NoErr(t, tx.Model(&models.GuestRecord{}).Where("id = ?", gid).Update("pax", 4).Error)
	assigned := make(chan error, 1)
	go func() { assigned <- svc.AssignSeat(t.Context(), gid, wid, tid, 9) }()
	time.Sleep(100 * time.Millisecond) // let AssignSeat block on the lock
	testutil.NoErr(t, tx.Commit().Error)

	// Seats 9-12 do not fit a 10-seat table.
	if err := <-assigned; !errors.Is(err, ErrSeatRange) {
		t.Fatalf("AssignSeat err = %v, want ErrSeatRange", err)
	}
}

// A notes edit that commits while an online check-in waits on the guest row
// must survive: the check-in appends to the committed notes, not a copy read
// before the edit.
func TestCheckIn_PostgresKeepsNotesCommittedWhileWaiting(t *testing.T) {
	svc, db := openGuestPG(t)
	wid, gid := uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "G", Pax: 1}).Error)

	// Hold the guest row the way a Drawer save does, while writing a note.
	tx := db.Begin()
	testutil.NoErr(t, tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", gid).First(&models.GuestRecord{}).Error)
	testutil.NoErr(t, tx.Model(&models.GuestRecord{}).Where("id = ?", gid).Update("notes", "halal").Error)
	checkedIn := make(chan error, 1)
	go func() { checkedIn <- svc.CheckIn(t.Context(), gid, wid, "came late") }()
	time.Sleep(100 * time.Millisecond) // let CheckIn block on the row lock
	testutil.NoErr(t, tx.Commit().Error)
	testutil.NoErr(t, <-checkedIn)

	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.Notes != "halal\ncame late" {
		t.Fatalf("notes = %q, want both notes", got.Notes)
	}
}

// A guest created while its wedding is being deleted must not outlive the
// wedding: the delete waits for the wedding lock and then removes the guest.
func TestWeddingDelete_PostgresLeavesNoOrphanGuest(t *testing.T) {
	_, db := openGuestPG(t)
	drop := func() { db.Exec("DROP TABLE IF EXISTS hall_elements, user_weddings CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.AutoMigrate(&models.HallElement{}, &models.UserWedding{}))
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)

	// A guest create holds the wedding lock, as guestCountLocked takes it.
	tx := db.Begin()
	_, err := guestCountLocked(tx, wid)
	testutil.NoErr(t, err)
	testutil.NoErr(t, tx.Create(&models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Late", Pax: 1}).Error)
	deleted := make(chan error, 1)
	go func() { deleted <- repository.NewWeddingRepo(db).Delete(t.Context(), wid) }()
	time.Sleep(100 * time.Millisecond) // let Delete block on the wedding lock
	testutil.NoErr(t, tx.Commit().Error)
	testutil.NoErr(t, <-deleted)

	var n int64
	testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&n).Error)
	if n != 0 {
		t.Fatalf("%d guest rows outlived their wedding", n)
	}
}
