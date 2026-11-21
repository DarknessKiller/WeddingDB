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
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, guest_tombstones, banquet_tables, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}, &models.GuestTombstone{}))
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

// A 200-character Han name romanizes to far more than 255 letters. A database
// migrated while name_pinyin was varchar(255) must be widened by AutoMigrate,
// or every such save is a 500 and a synced one stays queued forever.
func TestCreateGuest_PostgresLongHanNamePinyinFits(t *testing.T) {
	svc, db := openGuestPG(t)
	testutil.NoErr(t, db.Exec("ALTER TABLE guest_records ALTER COLUMN name_pinyin TYPE varchar(255)").Error)
	testutil.NoErr(t, db.AutoMigrate(&models.GuestRecord{}, &models.GuestTombstone{}))
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)

	g := &models.GuestRecord{WeddingID: wid, Name: strings.Repeat("庄", MaxGuestNameLen), Pax: 1}
	testutil.NoErr(t, svc.Create(t.Context(), g))
	if n := len(g.NamePinyin); n <= 255 {
		t.Fatalf("pinyin is %d bytes; the test needs one over 255", n)
	}
}

// holdGuestRow locks gid the way a concurrent sync check-in does and applies
// updates inside that transaction. Commit it to release the lock.
func holdGuestRow(t *testing.T, db *gorm.DB, gid uuid.UUID, updates map[string]any) *gorm.DB {
	t.Helper()
	tx := db.Begin()
	testutil.NoErr(t, tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", gid).First(&models.GuestRecord{}).Error)
	testutil.NoErr(t, tx.Model(&models.GuestRecord{}).Where("id = ?", gid).Updates(updates).Error)
	return tx
}

// A queued check-out older than a check-in that commits while the check-out
// waits must lose: it may not wipe the arrival or move updated_at backwards.
func TestSyncCheckOut_PostgresLosesToCheckInCommittedWhileWaiting(t *testing.T) {
	svc, db := openGuestPG(t)
	wid, gid := uuid.New(), uuid.New()
	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "G", Pax: 1, CheckedInAt: &t0, UpdatedAt: t0}).Error)

	checkedIn := t0.Add(20 * time.Minute)
	tx := holdGuestRow(t, db, gid, map[string]any{"checked_in_at": checkedIn, "updated_at": checkedIn})
	done := make(chan SyncResult, 1)
	go func() {
		res, err := svc.Sync(t.Context(), wid, []SyncMutation{{Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: t0.Add(10 * time.Minute)}})
		if err != nil {
			t.Errorf("sync: %v", err)
		}
		done <- res[0]
	}()
	time.Sleep(100 * time.Millisecond) // let the sync read the old row and block
	testutil.NoErr(t, tx.Commit().Error)
	if res := <-done; res.Status != "skipped" {
		t.Errorf("status = %q (%s), want skipped", res.Status, res.Reason)
	}

	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.CheckedInAt == nil || !got.UpdatedAt.Equal(checkedIn) {
		t.Fatalf("checked_in_at = %v updated_at = %v, want the newer check-in kept", got.CheckedInAt, got.UpdatedAt)
	}
}

// A queued delete made on a copy older than an edit that commits while the
// delete waits must be refused, not delete the guest with that edit unseen.
func TestSyncDelete_PostgresRefusesStaleBase(t *testing.T) {
	svc, db := openGuestPG(t)
	wid, gid := uuid.New(), uuid.New()
	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "G", Pax: 1, UpdatedAt: t0}).Error)

	tx := holdGuestRow(t, db, gid, map[string]any{"notes": "VIP uncle", "updated_at": t0.Add(time.Minute)})
	done := make(chan SyncResult, 1)
	go func() {
		res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
			Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: t0.Add(2 * time.Minute), BaseUpdatedAt: &t0,
		}})
		if err != nil {
			t.Errorf("sync: %v", err)
		}
		done <- res[0]
	}()
	time.Sleep(100 * time.Millisecond) // let the sync read the old row and block
	testutil.NoErr(t, tx.Commit().Error)
	if res := <-done; res.Status != "failed" || res.Retryable {
		t.Errorf("status = %q retryable=%v (%s), want a non-retryable failure", res.Status, res.Retryable, res.Reason)
	}
	testutil.NoErr(t, db.First(&models.GuestRecord{}, "id = ?", gid).Error)
}

// A synced create whose id another wedding's guest already holds can never
// succeed, so it must not be reported retryable and kept queued forever.
func TestSyncCreate_PostgresTakenIDIsNotRetryable(t *testing.T) {
	svc, db := openGuestPG(t)
	wid, other, gid := uuid.New(), uuid.New(), uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: other, Name: "O"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: other, Name: "Taken", Pax: 1}).Error)

	res, err := svc.Sync(t.Context(), wid, []SyncMutation{{
		Op: SyncOpCreate, GuestID: gid.String(), ClientUpdatedAt: time.Now(), Payload: &SyncPayload{Name: syncStr("New")},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" || res[0].Retryable {
		t.Fatalf("status = %q retryable=%v (%s), want a non-retryable failure", res[0].Status, res[0].Retryable, res[0].Reason)
	}
}

// A layout save waiting on the wedding lock must diff against the elements
// the lock holder committed: full replace removes them too.
func TestLayoutSave_PostgresReplacesElementsCommittedWhileWaiting(t *testing.T) {
	_, db := openGuestPG(t)
	drop := func() { db.Exec("DROP TABLE IF EXISTS hall_elements CASCADE") }
	drop()
	t.Cleanup(drop)
	testutil.NoErr(t, db.AutoMigrate(&models.HallElement{}))
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "W"}).Error)

	// Another save holds the wedding lock while adding an element.
	tx := db.Begin()
	_, err := guestCountLocked(tx, wid)
	testutil.NoErr(t, err)
	testutil.NoErr(t, tx.Create(&models.HallElement{WeddingID: wid, Type: "stage", Width: 1, Height: 1}).Error)
	saved := make(chan error, 1)
	go func() {
		saved <- NewLayoutService(repository.NewLayoutRepo(db)).Save(t.Context(), wid, 0, 0, nil, nil)
	}()
	time.Sleep(100 * time.Millisecond) // let Save block on the wedding lock
	testutil.NoErr(t, tx.Commit().Error)
	testutil.NoErr(t, <-saved)

	var n int64
	testutil.NoErr(t, db.Model(&models.HallElement{}).Where("wedding_id = ?", wid).Count(&n).Error)
	if n != 0 {
		t.Fatalf("%d elements survived a save that replaced them with none", n)
	}
}
