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

// A login that loses to a revoke (cache behind Postgres) had the right
// password: it must not come back as invalid credentials, which costs the
// user a lockout strike, and the next attempt must succeed.
func TestLogin_StaleVersionIsNotBadPassword(t *testing.T) {
	svc, mr, db := newIntegrationAuthService(t, "secret")
	id := uuid.New()
	hash, err := bcrypt.GenerateFromPassword([]byte("Passw0rd!x1"), bcrypt.MinCost)
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Create(&models.AdminUser{ID: id, Email: "s@x.test", Password: string(hash), Role: "user"}).Error)
	testutil.NoErr(t, db.Exec(`UPDATE admin_users SET token_version = 1 WHERE id = ?`, id).Error)
	testutil.NoErr(t, mr.Set(tvKey(id), "0")) // a revoke whose Redis write failed

	ctx := context.Background()
	if _, err := svc.Login(ctx, "s@x.test", "Passw0rd!x1"); !errors.Is(err, ErrSessionsChanged) {
		t.Fatalf("stale login: got %v, want ErrSessionsChanged", err)
	}
	if _, err := svc.Login(ctx, "s@x.test", "Passw0rd!x1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
}
