package services

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRefresh_UnknownTokenIsInvalid(t *testing.T) {
	svc, _, _ := newIntegrationAuthService(t, "secret")

	_, err := svc.Refresh(context.Background(), "no-such-token", "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("got %v, want ErrInvalidToken", err)
	}
}

// A Redis outage while minting the successor must not look like a dead token,
// or the client ends the session and drops its offline queue.
func TestRefresh_InfraFailureIsNotInvalidToken(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "secret")
	adminID := uuid.New()
	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, "valid-token")
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	sqlDB, err := broken.DB()
	testutil.NoErr(t, err)
	testutil.NoErr(t, sqlDB.Close())
	svc.adminRepo = repository.NewAdminRepo(broken)

	_, err = svc.Refresh(context.Background(), "valid-token", "")
	if err == nil {
		t.Fatal("expected refresh to fail while the account store is down")
	}
	if errors.Is(err, ErrInvalidToken) {
		t.Fatalf("infra failure classified as dead token: %v", err)
	}
}
