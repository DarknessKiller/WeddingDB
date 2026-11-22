package services

import (
	"context"
	"testing"
	"time"

	"weddingdb/internal/models"

	"github.com/google/uuid"
)

// A revoke deletes every refresh token and bumps the version, but a Rotate that
// already committed can leave a row behind. That row was minted under the old
// version and must be refused, not rotated into a live session.
func TestRefresh_RejectsStaleRefreshTokenAfterRevoke(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	ctx := context.Background()
	adminID := uuid.New()

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, "live-token")

	if err := svc.RevokeUserTokens(ctx, adminID); err != nil {
		t.Fatalf("RevokeUserTokens failed: %v", err)
	}
	// Simulate a Rotate that committed after the second delete.
	seedRefreshToken(t, db, adminID, nil, "stale-successor")

	if _, err := svc.Refresh(ctx, "stale-successor", ""); err == nil {
		t.Fatal("expected a refresh token minted under the pre-revoke version to be rejected")
	}
	var count int64
	db.Model(&models.RefreshToken{}).Where("token = ?", "stale-successor").Count(&count)
	if count != 0 {
		t.Fatalf("stale refresh token should be deleted, got %d rows", count)
	}
}

// The resume path serves a successor for a consumed token. A successor minted
// before the bump must be dropped instead of handed out.
func TestRefresh_RejectsStaleRotatedSuccessor(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	ctx := context.Background()
	adminID := uuid.New()

	seedAdmin(t, db, adminID, "admin")

	now := time.Now()
	replacedBy := "stale-replacement"
	consumed := &models.RefreshToken{
		ID:           uuid.New(),
		AdminID:      adminID,
		Token:        "consumed-token",
		ExpiresAt:    now.Add(time.Hour),
		RotatedAt:    &now,
		ReplacedBy:   &replacedBy,
		TokenVersion: 0,
	}
	if err := db.Create(consumed).Error; err != nil {
		t.Fatalf("failed to seed consumed token: %v", err)
	}
	if err := db.Create(&models.RefreshToken{
		ID:           uuid.New(),
		AdminID:      adminID,
		Token:        replacedBy,
		ExpiresAt:    now.Add(time.Hour),
		TokenVersion: 0,
	}).Error; err != nil {
		t.Fatalf("failed to seed replacement token: %v", err)
	}

	if err := svc.RevokeUserTokens(ctx, adminID); err != nil {
		t.Fatalf("RevokeUserTokens failed: %v", err)
	}
	// Revoke deletes the rows; reinsert them to model a Rotate that committed
	// after the bump (consumed row included so the resume path is reachable).
	if err := db.Create(consumed).Error; err != nil {
		t.Fatalf("failed to reinsert consumed token: %v", err)
	}
	if err := db.Create(&models.RefreshToken{
		ID:           uuid.New(),
		AdminID:      adminID,
		Token:        replacedBy,
		ExpiresAt:    now.Add(time.Hour),
		TokenVersion: 0,
	}).Error; err != nil {
		t.Fatalf("failed to reinsert replacement token: %v", err)
	}

	if _, err := svc.Refresh(ctx, consumed.Token, ""); err == nil {
		t.Fatal("expected the stale rotated successor to be rejected")
	}
	var count int64
	db.Model(&models.RefreshToken{}).Where("token = ?", replacedBy).Count(&count)
	if count != 0 {
		t.Fatalf("stale successor should be deleted, got %d rows", count)
	}
}

// The happy path still works and the successor carries the live version.
func TestRefresh_CurrentVersionSucceeds(t *testing.T) {
	svc, _, db := newIntegrationAuthService(t, "test-secret")
	ctx := context.Background()
	adminID := uuid.New()

	seedAdmin(t, db, adminID, "admin")
	seedRefreshToken(t, db, adminID, nil, "current-token")

	result, err := svc.Refresh(ctx, "current-token", "")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("expected a full set of tokens")
	}

	var successor models.RefreshToken
	if err := db.Where("token = ?", result.RefreshToken).First(&successor).Error; err != nil {
		t.Fatalf("failed to load successor: %v", err)
	}
	if successor.TokenVersion != 0 {
		t.Errorf("successor token_version = %d, want 0", successor.TokenVersion)
	}
}
