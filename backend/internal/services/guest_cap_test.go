package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

func seedGuestRows(t *testing.T, db *gorm.DB, wid uuid.UUID, n, pax int) {
	t.Helper()
	for i := 0; i < n; i++ {
		g := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: fmt.Sprintf("Guest %d", i), Pax: pax}
		if err := db.Create(g).Error; err != nil {
			t.Fatalf("seed guest: %v", err)
		}
	}
}

func TestGuestCap_Create(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	seedGuestRows(t, db, wid, 999, 1)

	if err := svc.Create(ctx, &models.GuestRecord{WeddingID: wid, Name: "Last", Pax: 1}); err != nil {
		t.Fatalf("guest 1000 should fit: %v", err)
	}
	err := svc.Create(ctx, &models.GuestRecord{WeddingID: wid, Name: "Over", Pax: 1})
	if !errors.Is(err, ErrGuestCapExceeded) {
		t.Fatalf("guest 1001 should be rejected, got %v", err)
	}
}

func TestGuestCap_CountsPartySize(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	seedGuestRows(t, db, wid, 998, 1)

	couple := &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: "Couple", Pax: 2}
	if err := svc.Create(ctx, couple); err != nil {
		t.Fatalf("couple should fit exactly: %v", err)
	}
	if err := svc.Create(ctx, &models.GuestRecord{WeddingID: wid, Name: "Over", Pax: 1}); !errors.Is(err, ErrGuestCapExceeded) {
		t.Fatalf("full wedding should reject another guest, got %v", err)
	}

	fetched, err := svc.Get(ctx, couple.ID, wid)
	if err != nil {
		t.Fatalf("get couple: %v", err)
	}
	fetched.Pax = 3
	if err := svc.Update(ctx, fetched); !errors.Is(err, ErrGuestCapExceeded) {
		t.Fatalf("growing pax past the cap should be rejected, got %v", err)
	}
	fetched.Pax = 1
	if err := svc.Update(ctx, fetched); err != nil {
		t.Fatalf("shrinking pax should fit: %v", err)
	}
}

func TestGuestCap_BulkImport(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	seedGuestRows(t, db, wid, 950, 1)

	batch := func(n int) []models.GuestRecord {
		out := make([]models.GuestRecord, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, models.GuestRecord{WeddingID: wid, Name: fmt.Sprintf("Import %d", i), Pax: 1})
		}
		return out
	}
	if _, err := svc.BulkCreate(ctx, batch(50)); err != nil {
		t.Fatalf("import to exactly 1000 should fit: %v", err)
	}
	if _, err := svc.BulkCreate(ctx, batch(1)); !errors.Is(err, ErrGuestCapExceeded) {
		t.Fatalf("import past the cap should be rejected, got %v", err)
	}
}

func TestGuestCap_SyncCreate(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	seedGuestRows(t, db, wid, 1000, 1)

	results, err := svc.Sync(ctx, wid, []SyncMutation{{
		MutationID:      uuid.New().String(),
		Op:              SyncOpCreate,
		GuestID:         uuid.New().String(),
		ClientUpdatedAt: time.Now(),
		Payload:         &SyncPayload{Name: "Late", Pax: 1},
	}})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(results) != 1 || results[0].Status != "failed" || !strings.Contains(results[0].Reason, "guest limit") {
		t.Fatalf("sync create past cap should fail with guest-limit reason, got %+v", results)
	}
}

func TestGuestCap_LegacyOverCapAllowsDecrease(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	seedGuestRows(t, db, wid, 1005, 1)

	if err := svc.Create(ctx, &models.GuestRecord{WeddingID: wid, Name: "Over", Pax: 1}); !errors.Is(err, ErrGuestCapExceeded) {
		t.Fatalf("adding to an over-cap wedding must fail, got %v", err)
	}

	var first models.GuestRecord
	if err := db.Where("wedding_id = ?", wid).First(&first).Error; err != nil {
		t.Fatalf("load first guest: %v", err)
	}
	first.Pax = 1
	if err := svc.Update(ctx, &first); err != nil {
		t.Fatalf("non-increasing update must be allowed: %v", err)
	}
	if err := svc.Delete(ctx, first.ID, wid); err != nil {
		t.Fatalf("delete must be allowed: %v", err)
	}
}
