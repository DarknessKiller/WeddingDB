package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Deleting a guest that does not exist must fail instead of reporting success
// (and broadcasting a delete for a zero-value guest).
func TestDelete_MissingGuestReturnsErrGuestNotFound(t *testing.T) {
	svc, _, wid := newGuestSyncService(t)

	if err := svc.Delete(context.Background(), uuid.New(), wid); !errors.Is(err, ErrGuestNotFound) {
		t.Fatalf("Delete missing guest = %v, want ErrGuestNotFound", err)
	}
}
