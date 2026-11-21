package services

import (
	"context"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// A token minted by a login that raced a user delete must stop working,
// both while the pin lasts and after it expires.
func TestPinDeletedUser_RejectsSurvivingToken(t *testing.T) {
	svc, mr, db := newIntegrationAuthService(t, "secret")
	id := uuid.New()
	seedAdmin(t, db, id, "admin")
	testutil.NoErr(t, mr.Set(tvKey(id), "1"))
	claims := &AccessClaims{AdminID: id, Role: "admin", TokenVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
		ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenTTL)),
	}}
	ctx := context.Background()
	testutil.NoErr(t, svc.CheckRevoked(ctx, claims))

	testutil.NoErr(t, db.Delete(&models.AdminUser{}, "id = ?", id).Error)
	testutil.NoErr(t, svc.PinDeletedUser(ctx, id))
	if err := svc.CheckRevoked(ctx, claims); err == nil {
		t.Fatal("token of deleted user still accepted")
	}
	mr.FastForward(accessTokenTTL + time.Second)
	if err := svc.CheckRevoked(ctx, claims); err == nil {
		t.Fatal("token of deleted user accepted after pin expired")
	}
}
