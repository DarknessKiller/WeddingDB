package services

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// newPGTestDB opens the Postgres test database and refuses to drop tables in a
// database whose name does not end in _test. Requires TEST_DATABASE_URL.
func newPGTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(pgDSN(t)), &gorm.Config{})
	if err != nil {
		t.Fatalf("postgres open: %v", err)
	}
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, banquet_tables, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestGuestWrite_MissingWeddingIsNotFoundOrApplied covers the two mappings the
// wedding row lock introduced: online guest writes answer the not-found
// sentinel (404), and an offline delete for a gone wedding reports applied
// instead of a retryable internal error.
func TestGuestWrite_MissingWeddingIsNotFoundOrApplied(t *testing.T) {
	db := newPGTestDB(t)
	svc := NewGuestService(repository.NewGuestRepo(db), nil, db)
	ctx := t.Context()
	goneWedding := uuid.New()
	guestID := uuid.New()

	if err := svc.Delete(ctx, guestID, goneWedding); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("delete in a missing wedding err = %v, want ErrGuestNotFound", err)
	}
	if _, err := svc.CheckIn(ctx, guestID, goneWedding, ""); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("check-in in a missing wedding err = %v, want ErrGuestNotFound", err)
	}
	if err := svc.CheckOut(ctx, guestID, goneWedding); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("check-out in a missing wedding err = %v, want ErrGuestNotFound", err)
	}
	if err := svc.Create(ctx, &models.GuestRecord{ID: guestID, WeddingID: goneWedding, Name: "A", Pax: 1}); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("create in a missing wedding err = %v, want ErrGuestNotFound", err)
	}
	if err := svc.AssignSeat(ctx, guestID, goneWedding, uuid.New(), 1); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("assign seat in a missing wedding err = %v, want ErrGuestNotFound", err)
	}
	if _, err := svc.BulkCreate(ctx, []models.GuestRecord{{ID: uuid.New(), WeddingID: goneWedding, Name: "B", Pax: 1}}); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("bulk create in a missing wedding err = %v, want ErrGuestNotFound", err)
	}

	res, err := svc.Sync(ctx, goneWedding, []SyncMutation{{
		Op: SyncOpDelete, GuestID: guestID.String(), ClientUpdatedAt: time.Now(),
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "applied" {
		t.Fatalf("offline delete for a missing wedding = %+v, want applied", res[0])
	}

	var payload SyncPayload
	testutil.NoErr(t, json.Unmarshal([]byte(`{"name":"X"}`), &payload))
	res, err = svc.Sync(ctx, goneWedding, []SyncMutation{{
		Op: SyncOpCreate, GuestID: uuid.New().String(), ClientUpdatedAt: time.Now(), Payload: &payload,
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" || res[0].Retryable == nil || *res[0].Retryable {
		t.Fatalf("offline create for a missing wedding = %+v, want failed without retry", res[0])
	}
}
