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
)

// openAuditPG opens TEST_DATABASE_URL and migrates the tables the locked
// write paths below touch.
func openAuditPG(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(pgDSN(t)), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to write to %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	testutil.NoErr(t, db.AutoMigrate(&models.WeddingEvent{}, &models.AdminUser{}, &models.BanquetTable{},
		&models.GuestRecord{}, &models.HallElement{}, &models.AppliedMutation{}, &models.UserWedding{}))
	return db
}

// Two editors save from the same loaded version at once: exactly one wins.
func TestLayoutVersion_PostgresConcurrent(t *testing.T) {
	db := openAuditPG(t)
	wid := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Layout PG"}).Error)
	repo := repository.NewLayoutRepo(db)
	v := 0

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = repo.SaveLayout(t.Context(), wid, &v, 900, 900, nil, nil, nil, nil)
		})
	}
	wg.Wait()
	conflicts := 0
	for _, err := range errs {
		if errors.Is(err, repository.ErrLayoutVersionConflict) {
			conflicts++
		} else {
			testutil.NoErr(t, err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want exactly 1 (errs %v)", conflicts, errs)
	}
}

// Two admins demote each other at once: one admin must remain.
func TestLastAdmin_PostgresConcurrent(t *testing.T) {
	db := openAuditPG(t)
	testutil.NoErr(t, db.Exec("DELETE FROM user_weddings").Error)
	testutil.NoErr(t, db.Exec("DELETE FROM admin_users").Error)
	a, b := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{a, b} {
		testutil.NoErr(t, db.Create(&models.AdminUser{ID: id, Email: id.String() + "@x.test", Password: "h", Role: "admin"}).Error)
	}
	repo := repository.NewAdminRepo(db)

	var wg sync.WaitGroup
	for _, id := range []uuid.UUID{a, b} {
		wg.Go(func() {
			if err := repo.UpdateRole(t.Context(), id, "user"); err != nil && !errors.Is(err, repository.ErrLastAdmin) {
				t.Errorf("demote %s: %v", id, err)
			}
		})
	}
	wg.Wait()
	var admins int64
	testutil.NoErr(t, db.Model(&models.AdminUser{}).Where("role = ?", "admin").Count(&admins).Error)
	if admins != 1 {
		t.Fatalf("admins left = %d, want 1", admins)
	}
}

// A replayed check-in on Postgres reports applied and appends its note once.
func TestSyncReplay_Postgres(t *testing.T) {
	db := openAuditPG(t)
	wid, gid := uuid.New(), uuid.New()
	base := time.Now().Add(-time.Hour)
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Sync PG"}).Error)
	testutil.NoErr(t, db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Ann", Pax: 1, CreatedAt: base, UpdatedAt: base}).Error)
	svc := NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, db)
	m := SyncMutation{MutationID: uuid.NewString(), Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: base.Add(time.Minute), Payload: &SyncPayload{Notes: "late"}}

	for i := range 2 {
		res, err := svc.Sync(t.Context(), wid, []SyncMutation{m})
		testutil.NoErr(t, err)
		if res[0].Status != "applied" {
			t.Fatalf("run %d: %+v", i, res[0])
		}
	}
	var got models.GuestRecord
	testutil.NoErr(t, db.First(&got, "id = ?", gid).Error)
	if got.Notes != "late" {
		t.Fatalf("notes = %q, want one note", got.Notes)
	}
}
