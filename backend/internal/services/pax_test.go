package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
)

// A single guest entry above the party-size cap must be rejected on every
// write path, not silently counted past the wedding limit.
func TestCreate_RejectsPaxOverLimit(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)

	err := svc.Create(context.Background(), &models.GuestRecord{
		WeddingID: wid,
		Name:      "Big Party",
		Pax:       MaxGuestPartySize + 1,
	})
	if !errors.Is(err, ErrInvalidPax) {
		t.Fatalf("Create over-limit pax error = %v, want ErrInvalidPax", err)
	}
}

func TestUpdate_RejectsPaxOverLimit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Small", Pax: 2})

	guest, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	guest.Pax = MaxGuestPartySize + 1
	if err := svc.Update(ctx, guest); !errors.Is(err, ErrInvalidPax) {
		t.Fatalf("Update over-limit pax error = %v, want ErrInvalidPax", err)
	}
}

func TestBulkCreate_RejectsPaxOverLimit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)

	_, err := svc.BulkCreate(context.Background(), []models.GuestRecord{
		{WeddingID: wid, Name: "Fine", Pax: 2},
		{WeddingID: wid, Name: "Too Big", Pax: MaxGuestPartySize + 1},
	})
	if !errors.Is(err, ErrInvalidPax) {
		t.Fatalf("BulkCreate over-limit pax error = %v, want ErrInvalidPax", err)
	}
	var count int64
	db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).Count(&count)
	if count != 0 {
		t.Fatalf("bulk import over-limit should create nothing, got %d rows", count)
	}
}

func TestSync_RejectsPaxOverLimit(t *testing.T) {
	svc, db, wid := newGuestSyncService(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Minute)
	gid := uuid.New()
	db.Create(&models.GuestRecord{ID: gid, WeddingID: wid, Name: "Small", Pax: 2, CreatedAt: base, UpdatedAt: base})

	res, err := svc.Sync(ctx, wid, []SyncMutation{{
		Op:              SyncOpUpdate,
		GuestID:         gid.String(),
		ClientUpdatedAt: base.Add(time.Second),
		Payload:         &SyncPayload{Pax: syncInt(MaxGuestPartySize + 1)},
	}})
	testutil.NoErr(t, err)
	if res[0].Status != "failed" {
		t.Fatalf("sync over-limit pax status = %q, want failed", res[0].Status)
	}

	g, err := svc.Get(ctx, gid, wid)
	testutil.NoErr(t, err)
	if g.Pax != 2 {
		t.Fatalf("sync over-limit pax changed stored pax to %d, want 2", g.Pax)
	}
}
