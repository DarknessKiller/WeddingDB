package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
)

// A failure while minting used to consume the refresh token with nothing issued
// in its place, which logged the user out for good. Minting now happens before
// the consume, so the token survives and the retry succeeds.
func TestRefresh_MintFailureKeepsTokenUsable(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	refreshStr := "grace-mint-failure"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, refreshStr)

	// Simulate a token-version read failure: generateAccessToken must fail
	// before Rotate consumes the presented token.
	testutil.NoErr(t, db.Exec("ALTER TABLE admin_users DROP COLUMN token_version").Error)

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err == nil {
		t.Fatal("expected Refresh to fail while the token version is unreadable")
	}

	var live int64
	db.Model(&models.RefreshToken{}).Where("token = ? AND rotated_at IS NULL", repository.HashToken(refreshStr)).Count(&live)
	if live != 1 {
		t.Fatalf("failed Refresh consumed the token: %d unrotated rows left", live)
	}

	testutil.NoErr(t, db.Exec("ALTER TABLE admin_users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0").Error)

	result, err := svc.Refresh(context.Background(), refreshStr, "")
	if err != nil {
		t.Fatalf("expected the untouched token to still work after recovery, got: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("expected a full set of tokens after recovery")
	}
}

// Two tabs refreshing at the same instant must both end up with a session.
func TestRefresh_ConcurrentRefreshesShareSuccessor(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	refreshStr := "grace-concurrent"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, refreshStr)

	var wg sync.WaitGroup
	tokens := make([]string, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := svc.Refresh(context.Background(), refreshStr, "")
			if err != nil {
				errs[i] = err
				return
			}
			tokens[i] = res.RefreshToken
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Refresh %d failed: %v", i, err)
		}
	}
	if tokens[0] != tokens[1] {
		t.Errorf("concurrent refreshes returned different successors: %q vs %q", tokens[0], tokens[1])
	}
}

// Replay protection still holds outside the grace window.
func TestRefresh_ReplayAfterGraceRejected(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	refreshStr := "grace-expired"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, refreshStr)

	first, err := svc.Refresh(context.Background(), refreshStr, "")
	if err != nil {
		t.Fatalf("first Refresh failed: %v", err)
	}

	db.Model(&models.RefreshToken{}).Where("token = ?", repository.HashToken(refreshStr)).
		Update("rotated_at", time.Now().Add(-refreshGraceWindow-time.Minute))

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err == nil {
		t.Fatal("expected a replay outside the grace window to be rejected")
	}
	// Reuse detection ends every session: the successor and access tokens die.
	if _, err := svc.Refresh(context.Background(), first.RefreshToken, ""); err == nil {
		t.Fatal("successor still usable after reuse was detected")
	}
	if _, err := svc.ValidateToken(context.Background(), first.AccessToken); err == nil {
		t.Fatal("access token still valid after reuse was detected")
	}
}

// A replayed token whose successor was already consumed is reuse too, even
// inside the grace window: otherwise a thief pacing the victim's rotations
// keeps minting access tokens forever.
func TestRefresh_ReplayWithConsumedSuccessorRejectedInsideGrace(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	refreshStr := "grace-chain"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, refreshStr)

	first, err := svc.Refresh(context.Background(), refreshStr, "")
	if err != nil {
		t.Fatalf("first Refresh failed: %v", err)
	}
	second, err := svc.Refresh(context.Background(), first.RefreshToken, "")
	if err != nil {
		t.Fatalf("successor Refresh failed: %v", err)
	}

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err == nil {
		t.Fatal("replay of a token whose successor was consumed was accepted")
	}
	if _, err := svc.Refresh(context.Background(), second.RefreshToken, ""); err == nil {
		t.Fatal("live successor still usable after reuse was detected")
	}
	if _, err := svc.ValidateToken(context.Background(), second.AccessToken); err == nil {
		t.Fatal("access token still valid after reuse was detected")
	}
}

// The claim is exclusive and the loser's replacement is not stored.
func TestTokenRotate_SecondClaimRejected(t *testing.T) {
	_, _, db := newIntegrationAuthService(t, "test-secret")
	repo := repository.NewTokenRepo(db)
	adminID := uuid.New()
	seedRefreshToken(t, db, adminID, nil, "claim-token")

	winner := &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "claim-successor", ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Rotate(context.Background(), "claim-token", winner); err != nil {
		t.Fatalf("first Rotate failed: %v", err)
	}

	loser := &models.RefreshToken{ID: uuid.New(), AdminID: adminID, Token: "claim-loser", ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Rotate(context.Background(), "claim-token", loser); !errors.Is(err, repository.ErrTokenClaimed) {
		t.Fatalf("expected ErrTokenClaimed from the second Rotate, got: %v", err)
	}

	var stored int64
	db.Model(&models.RefreshToken{}).Where("token = ?", repository.HashToken("claim-loser")).Count(&stored)
	if stored != 0 {
		t.Errorf("loser's replacement was stored: %d rows", stored)
	}
	var live int64
	db.Model(&models.RefreshToken{}).Where("token = ? AND rotated_at IS NULL", repository.HashToken("claim-successor")).Count(&live)
	if live != 1 {
		t.Errorf("winner's replacement missing: %d rows", live)
	}
}

// Refresh tokens rest as hashes: the plaintext never reaches the table, and
// rotation and lookup still work through the hash.
func TestRefreshToken_StoredHashed(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	seedAdmin(t, db, adminID, "admin")

	issued, err := svc.generateRefreshToken(t.Context(), adminID, nil)
	testutil.NoErr(t, err)
	res, err := svc.Refresh(t.Context(), issued.Token, "")
	testutil.NoErr(t, err)

	for _, plain := range []string{issued.Token, res.RefreshToken} {
		var n int64
		testutil.NoErr(t, db.Model(&models.RefreshToken{}).Where("token = ? OR replaced_by = ?", plain, plain).Count(&n).Error)
		if n != 0 {
			t.Fatalf("plaintext token %q found in refresh_tokens", plain)
		}
	}
	var hashedRows int64
	testutil.NoErr(t, db.Model(&models.RefreshToken{}).Where("token IN ?", []string{repository.HashToken(issued.Token), repository.HashToken(res.RefreshToken)}).Count(&hashedRows).Error)
	if hashedRows != 2 {
		t.Fatalf("hashed rows = %d, want 2", hashedRows)
	}
}
