package services

import (
	"testing"

	"weddingdb/internal/models"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
)

// The access token carries the user's force-password-change flag so the auth
// middleware can enforce it without a DB lookup per request.
func TestAccessToken_CarriesForcePasswordChange(t *testing.T) {
	svc, _, _ := newIntegrationAuthService(t, "test-secret")

	for _, flagged := range []bool{true, false} {
		admin := &models.AdminUser{ID: uuid.New(), Role: "user", ForcePasswordChange: flagged}
		token, err := svc.generateAccessToken(t.Context(), admin, nil)
		testutil.NoErr(t, err)

		claims, err := svc.ValidateToken(t.Context(), token)
		testutil.NoErr(t, err)
		if claims.ForcePasswordChange != flagged {
			t.Errorf("flag %v: claim = %v", flagged, claims.ForcePasswordChange)
		}
	}
}

// CheckRevoked is what open SSE streams poll; it must catch both a blacklisted
// JTI and a bumped token version, and pass an untouched token.
func TestCheckRevoked(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	ctx := t.Context()

	mint := func(id uuid.UUID) *AccessClaims {
		t.Helper()
		token, err := svc.generateAccessToken(ctx, &models.AdminUser{ID: id, Role: "user"}, nil)
		testutil.NoErr(t, err)
		claims, err := svc.ValidateToken(ctx, token)
		testutil.NoErr(t, err)
		return claims
	}

	fresh := mint(uuid.New())
	if err := svc.CheckRevoked(ctx, fresh); err != nil {
		t.Fatalf("fresh token reported revoked: %v", err)
	}

	blacklisted := mint(uuid.New())
	testutil.NoErr(t, svc.BlacklistAccessToken(ctx, blacklisted))
	if svc.CheckRevoked(ctx, blacklisted) == nil {
		t.Error("blacklisted token passed CheckRevoked")
	}

	userID := uuid.New()
	seedAdmin(t, db, userID, "user")
	stale := mint(userID)
	testutil.NoErr(t, svc.RevokeUserTokens(ctx, userID))
	if svc.CheckRevoked(ctx, stale) == nil {
		t.Error("token from before RevokeUserTokens passed CheckRevoked")
	}
}
