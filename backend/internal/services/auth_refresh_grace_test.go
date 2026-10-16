package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

// A failure while minting used to consume the refresh token with nothing issued
// in its place, which logged the user out for good. Minting now happens before
// the consume, so the token survives and the retry succeeds.
func TestRefresh_MintFailureKeepsTokenUsable(t *testing.T) {
	svc, mr, db := newIntegrationAuthService(t, "test-secret")
	adminID := uuid.New()
	refreshStr := "grace-mint-failure"

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, refreshStr)

	addr := mr.Addr()
	mr.Close() // generateAccessToken -> GetTokenVersion now fails

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err == nil {
		t.Fatal("expected Refresh to fail while Redis is down")
	}

	var live int64
	db.Model(&models.RefreshToken{}).Where("token = ? AND rotated_at IS NULL", refreshStr).Count(&live)
	if live != 1 {
		t.Fatalf("failed Refresh consumed the token: %d unrotated rows left", live)
	}

	restarted := miniredis.NewMiniRedis()
	if err := restarted.StartAddr(addr); err != nil {
		t.Fatalf("restart miniredis: %v", err)
	}
	t.Cleanup(restarted.Close)

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

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err != nil {
		t.Fatalf("first Refresh failed: %v", err)
	}

	db.Model(&models.RefreshToken{}).Where("token = ?", refreshStr).
		Update("rotated_at", time.Now().Add(-refreshGraceWindow-time.Minute))

	if _, err := svc.Refresh(context.Background(), refreshStr, ""); err == nil {
		t.Fatal("expected a replay outside the grace window to be rejected")
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
	db.Model(&models.RefreshToken{}).Where("token = ?", "claim-loser").Count(&stored)
	if stored != 0 {
		t.Errorf("loser's replacement was stored: %d rows", stored)
	}
	var live int64
	db.Model(&models.RefreshToken{}).Where("token = ? AND rotated_at IS NULL", "claim-successor").Count(&live)
	if live != 1 {
		t.Errorf("winner's replacement missing: %d rows", live)
	}
}
