package services

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestSelectWedding_SentinelErrors(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "secret")
	ctx := context.Background()

	if _, err := svc.SelectWedding(ctx, uuid.New(), uuid.New()); !errors.Is(err, ErrAdminNotFound) {
		t.Fatalf("unknown admin: got %v, want ErrAdminNotFound", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	user := &models.AdminUser{ID: uuid.New(), Email: "u@x.test", Password: string(hash), Role: "user"}
	testutil.NoErr(t, db.Create(user).Error)
	if _, err := svc.SelectWedding(ctx, user.ID, uuid.New()); !errors.Is(err, ErrNoWeddingAccess) {
		t.Fatalf("no access: got %v, want ErrNoWeddingAccess", err)
	}
}
