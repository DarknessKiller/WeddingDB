package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// The edit form carries no checked_in_at. If another phone checked the guest
// in after this editor loaded the row, the broadcast must still show the
// committed arrival, or every client clears it.
func TestUpdate_BroadcastsCommittedCheckedInAt(t *testing.T) {
	_, db, wid := newGuestSyncService(t)
	mr := miniredis.RunT(t)
	hub := NewSSEHub(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	t.Cleanup(hub.Shutdown)
	svc := NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), hub, db)

	arrived := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	gid := uuid.New()
	testutil.NoErr(t, db.Create(&models.GuestRecord{
		ID: gid, WeddingID: wid, Name: "Ann", Pax: 1, CheckedInAt: &arrived,
	}).Error)

	client, ok := hub.Subscribe(wid, uuid.New())
	if !ok {
		t.Fatal("Subscribe rejected the first client")
	}
	// The editor's copy predates the check-in, so its arrival is nil.
	stale := &models.GuestRecord{ID: gid, WeddingID: wid, Name: "Ann Tan", Pax: 1}
	testutil.NoErr(t, svc.Update(context.Background(), stale))

	select {
	case raw := <-client.Chan:
		var ev GuestEvent
		testutil.NoErr(t, json.Unmarshal(raw, &ev))
		if ev.Guest == nil || ev.Guest.CheckedInAt == nil {
			t.Fatalf("broadcast guest = %+v, want the committed arrival", ev.Guest)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the update event")
	}
}
