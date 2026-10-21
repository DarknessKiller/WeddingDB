package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AccessClaims struct {
	AdminID      uuid.UUID  `json:"sub"`
	WeddingID    *uuid.UUID `json:"wid,omitempty"`
	Role         string     `json:"role"`
	TokenVersion int        `json:"tv"`
	jwt.RegisteredClaims
}

type AuthService struct {
	adminRepo   *repository.AdminRepo
	weddingRepo *repository.WeddingRepo
	tokenRepo   *repository.TokenRepo
	secret      []byte
	redisClient *redis.Client
}

func NewAuthService(
	adminRepo *repository.AdminRepo,
	weddingRepo *repository.WeddingRepo,
	tokenRepo *repository.TokenRepo,
	secret string,
	redisClient *redis.Client,
) *AuthService {
	return &AuthService{
		adminRepo:   adminRepo,
		weddingRepo: weddingRepo,
		tokenRepo:   tokenRepo,
		secret:      []byte(secret),
		redisClient: redisClient,
	}
}

// LoginResult holds the data returned by Login.
type LoginResult struct {
	AccessToken         string
	RefreshToken        string
	Role                string
	Name                string
	Weddings            []models.WeddingEvent
	ForcePasswordChange bool
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	admin, err := s.adminRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	accessToken, err := s.generateAccessToken(ctx, admin, nil)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.generateRefreshToken(ctx, admin.ID, nil)
	if err != nil {
		return nil, err
	}
	return s.sessionResult(ctx, admin, accessToken, refreshToken)
}

// sessionResult assembles the token payload shared by Login and Refresh.
func (s *AuthService) sessionResult(ctx context.Context, admin *models.AdminUser, accessToken, refreshToken string) (*LoginResult, error) {
	var weddings []models.WeddingEvent
	if admin.Role == "admin" {
		weddings, _ = s.weddingRepo.List(ctx)
	} else {
		weddings, _ = s.adminRepo.GetUserWeddings(ctx, admin.ID)
	}
	if weddings == nil {
		weddings = []models.WeddingEvent{}
	}
	return &LoginResult{
		AccessToken:         accessToken,
		RefreshToken:        refreshToken,
		Role:                admin.Role,
		Name:                admin.Name,
		Weddings:            weddings,
		ForcePasswordChange: admin.ForcePasswordChange,
	}, nil
}

// SelectWedding generates a new access token with the selected wedding embedded.
func (s *AuthService) SelectWedding(ctx context.Context, adminID uuid.UUID, weddingID uuid.UUID) (string, error) {
	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return "", errors.New("admin not found")
	}
	// Verify access
	if admin.Role != "admin" {
		hasAccess, err := s.adminRepo.HasWeddingAccess(ctx, adminID, weddingID)
		if err != nil || !hasAccess {
			return "", errors.New("no access to this wedding")
		}
		// Persist the wedding scope onto the caller's refresh token(s) so that
		// Refresh re-mints a scoped access token. Without this, the first
		// refresh after SelectWedding drops the wedding and the user gets 403
		// until they log in again. Admins keep a nil scope (all weddings).
		if err := s.tokenRepo.UpdateWeddingScope(ctx, adminID, weddingID); err != nil {
			return "", fmt.Errorf("failed to persist wedding scope: %w", err)
		}
	}
	return s.generateAccessToken(ctx, admin, &weddingID)
}

// refreshGraceWindow is how long after a rotation a retry of the consumed token
// is answered with the same successor. It covers responses lost in transit and
// refreshes racing from two tabs, without which either one ends the session.
const refreshGraceWindow = 60 * time.Second

// Refresh returns a new LoginResult with fresh tokens.
// oldAccessToken is the previous access token (from Authorization header) — its JTI is blacklisted
// to prevent concurrent use after rotation.
func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string, oldAccessToken string) (*LoginResult, error) {
	// Blacklist old access token JTI first — failure here leaves no token state mutated
	if oldAccessToken != "" {
		if oldClaims, err := s.parseTokenClaims(oldAccessToken); err == nil && oldClaims != nil {
			if err := s.BlacklistAccessToken(ctx, oldClaims); err != nil {
				return nil, fmt.Errorf("failed to blacklist old access token: %w", err)
			}
		}
	}

	token, err := s.tokenRepo.FindByToken(ctx, refreshTokenStr)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if token.RotatedAt != nil {
		return s.resumeRotation(ctx, token)
	}

	admin, err := s.adminRepo.FindByID(ctx, token.AdminID)
	if err != nil {
		return nil, errors.New("admin not found")
	}

	// Mint before consuming: if Redis or the token generator fails here, the
	// presented token stays usable and the client can simply retry.
	// Re-check wedding scope: the grant may have been revoked since mint.
	weddingID, err := s.activeWeddingScope(ctx, admin.ID, token.WeddingID)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.generateAccessToken(ctx, admin, weddingID)
	if err != nil {
		return nil, err
	}
	replacement, err := s.buildRefreshToken(admin.ID, weddingID)
	if err != nil {
		return nil, err
	}

	err = s.tokenRepo.Rotate(ctx, refreshTokenStr, replacement)
	if err == nil {
		return s.sessionResult(ctx, admin, accessToken, replacement.Token)
	}
	if !errors.Is(err, repository.ErrTokenClaimed) {
		return nil, fmt.Errorf("failed to rotate refresh token: %w", err)
	}
	// Another caller rotated it between our read and our claim — serve the
	// successor instead of failing one of the two.
	current, findErr := s.tokenRepo.FindByToken(ctx, refreshTokenStr)
	if findErr != nil || current.RotatedAt == nil {
		return nil, errors.New("invalid refresh token")
	}
	return s.resumeRotation(ctx, current)
}

// resumeRotation answers a retry of an already-rotated token with the same
// successor, inside the grace window. Outside it the token counts as replayed.
func (s *AuthService) resumeRotation(ctx context.Context, consumed *models.RefreshToken) (*LoginResult, error) {
	if consumed.RotatedAt == nil || consumed.ReplacedBy == nil || time.Since(*consumed.RotatedAt) > refreshGraceWindow {
		return nil, errors.New("invalid refresh token")
	}
	replacement, err := s.tokenRepo.FindByToken(ctx, *consumed.ReplacedBy)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	admin, err := s.adminRepo.FindByID(ctx, replacement.AdminID)
	if err != nil {
		return nil, errors.New("admin not found")
	}
	weddingID, err := s.activeWeddingScope(ctx, admin.ID, replacement.WeddingID)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.generateAccessToken(ctx, admin, weddingID)
	if err != nil {
		return nil, err
	}
	return s.sessionResult(ctx, admin, accessToken, replacement.Token)
}

// activeWeddingScope returns weddingID only while the user still has access to
// that wedding; a revoked grant degrades the session to unscoped.
func (s *AuthService) activeWeddingScope(ctx context.Context, userID uuid.UUID, weddingID *uuid.UUID) (*uuid.UUID, error) {
	if weddingID == nil {
		return nil, nil
	}
	ok, err := s.adminRepo.HasWeddingAccess(ctx, userID, *weddingID)
	if err != nil {
		return nil, fmt.Errorf("failed to check wedding access: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return weddingID, nil
}

// CheckLoginRate / CheckRegisterRate / CheckRefreshRate enforce fixed-window
// failure limits. checkRateLimit fails open: a Redis outage must not lock
// every user out, and sustained brute force through an outage is visible.
func (s *AuthService) CheckLoginRate(ctx context.Context, ip, email string) error {
	if err := s.checkRateLimit(ctx, "login:ip:"+ip, 10, 5*time.Minute); err != nil {
		return err
	}
	return s.checkRateLimit(ctx, "login:acct:"+strings.ToLower(email), 5, 15*time.Minute)
}

func (s *AuthService) RecordLoginFailure(ctx context.Context, ip, email string) {
	s.recordFailure(ctx, "login:ip:"+ip, 5*time.Minute)
	s.recordFailure(ctx, "login:acct:"+strings.ToLower(email), 15*time.Minute)
}

func (s *AuthService) CheckRegisterRate(ctx context.Context, ip string) error {
	return s.checkRateLimit(ctx, "register:ip:"+ip, 10, time.Hour)
}

func (s *AuthService) CheckRefreshRate(ctx context.Context, ip string) error {
	return s.checkRateLimit(ctx, "refresh:ip:"+ip, 60, 5*time.Minute)
}

func (s *AuthService) checkRateLimit(ctx context.Context, key string, limit int64, window time.Duration) error {
	n, err := s.redisClient.Get(ctx, "ratelimit:"+key).Int64()
	if err != nil { // redis.Nil = under limit; any other error = fail open
		return nil
	}
	if n >= limit {
		return errors.New("too many attempts, please try again later")
	}
	return nil
}

func (s *AuthService) recordFailure(ctx context.Context, key string, window time.Duration) {
	n, err := s.redisClient.Incr(ctx, "ratelimit:"+key).Result()
	if err != nil {
		return
	}
	if n == 1 {
		s.redisClient.Expire(ctx, "ratelimit:"+key, window)
	}
}

// Logout deletes the refresh token and blacklists the access token's JTI.
// Blacklisting is best-effort: a Redis failure does not prevent refresh token deletion.
func (s *AuthService) Logout(ctx context.Context, refreshToken string, accessToken string) error {
	// Blacklist the access token JTI (best-effort — don't block logout on Redis failure)
	if accessToken != "" {
		if claims, err := s.parseTokenClaims(accessToken); err == nil && claims != nil {
			s.BlacklistAccessToken(ctx, claims)
		}
	}
	// Best-effort delete: logging out with an already-consumed token is not an error.
	_, err := s.tokenRepo.DeleteByToken(ctx, refreshToken)
	return err
}

// ValidateToken parses and validates a JWT, then checks blacklist and token version.
func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Reject anything that isn't HMAC-SHA256
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Enforce required claims
	if claims.AdminID == uuid.Nil {
		return nil, errors.New("token missing admin ID")
	}
	if claims.Role == "" {
		return nil, errors.New("token missing role")
	}
	if claims.ID == "" {
		return nil, errors.New("token missing JTI")
	}
	if claims.ExpiresAt == nil {
		return nil, errors.New("token missing expiration")
	}

	// Check blacklist (targeted, time-bounded check — do this first)
	blacklisted, err := s.IsTokenBlacklisted(ctx, claims.ID)
	if err != nil {
		// Fail-closed: Redis error = reject
		return nil, fmt.Errorf("service temporarily unavailable: %w", err)
	}
	if blacklisted {
		return nil, errors.New("token has been revoked")
	}

	// Check token version (broader sweep)
	storedTV, err := s.GetTokenVersion(ctx, claims.AdminID)
	if err != nil {
		// Fail-closed: Redis error = reject
		return nil, fmt.Errorf("service temporarily unavailable: %w", err)
	}
	if claims.TokenVersion < storedTV {
		return nil, errors.New("token version is stale")
	}

	return claims, nil
}

// BlacklistAccessToken adds the JTI to the Redis blacklist with TTL = remaining token life.
func (s *AuthService) BlacklistAccessToken(ctx context.Context, claims *AccessClaims) error {
	if claims.ExpiresAt == nil || claims.ID == "" {
		return errors.New("cannot blacklist token: missing expiration or JTI")
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		// Token already expired, no need to blacklist
		return nil
	}
	key := fmt.Sprintf("blacklist:jti:%s", claims.ID)
	return s.redisClient.Set(ctx, key, "1", ttl).Err()
}

// IsTokenBlacklisted checks if a JTI is in the Redis blacklist.
func (s *AuthService) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	key := fmt.Sprintf("blacklist:jti:%s", jti)
	exists, err := s.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// GetTokenVersion returns the current token version for a user from Redis.
// Returns 0 if the key doesn't exist (lazy init: default tv=0).
func (s *AuthService) GetTokenVersion(ctx context.Context, userID uuid.UUID) (int, error) {
	key := fmt.Sprintf("user:%s:tv", userID.String())
	val, err := s.redisClient.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		// Key doesn't exist — user has never been revoked, tv=0
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

// RevokeUserTokens bumps the token version in Redis and deletes all refresh tokens for the user.
// Delete first, then INCR — closes the TOCTOU race where an attacker refreshes between INCR and delete.
func (s *AuthService) RevokeUserTokens(ctx context.Context, userID uuid.UUID) error {
	// Step 1: Delete refresh tokens from Postgres first.
	// Any concurrent Refresh call after this point will fail at FindByToken.
	if err := s.tokenRepo.DeleteByAdminID(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete refresh tokens: %w", err)
	}
	// Step 2: Bump token version. Any existing access token with old tv is now rejected by middleware.
	// Retry on transient Redis failures — can't undo the Postgres delete.
	key := fmt.Sprintf("user:%s:tv", userID.String())
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.redisClient.Incr(ctx, key).Err(); err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return fmt.Errorf("failed to increment token version after retries: %w", lastErr)
	}
	return nil
}

// parseTokenClaims parses a JWT without blacklist/tv checks — used only for extracting claims during logout.
func (s *AuthService) parseTokenClaims(tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *AuthService) generateAccessToken(ctx context.Context, admin *models.AdminUser, weddingID *uuid.UUID) (string, error) {
	tv, err := s.GetTokenVersion(ctx, admin.ID)
	if err != nil {
		return "", fmt.Errorf("failed to read token version: %w", err)
	}

	now := time.Now()
	expiry := now.Add(15 * time.Minute)
	claims := AccessClaims{
		AdminID:      admin.ID,
		WeddingID:    weddingID,
		Role:         admin.Role,
		TokenVersion: tv,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// buildRefreshToken returns an unsaved refresh token row.
func (s *AuthService) buildRefreshToken(adminID uuid.UUID, weddingID *uuid.UUID) (*models.RefreshToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &models.RefreshToken{
		ID:        uuid.New(),
		AdminID:   adminID,
		WeddingID: weddingID,
		Token:     hex.EncodeToString(b),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}, nil
}

func (s *AuthService) generateRefreshToken(ctx context.Context, adminID uuid.UUID, weddingID *uuid.UUID) (string, error) {
	token, err := s.buildRefreshToken(adminID, weddingID)
	if err != nil {
		return "", err
	}
	if err := s.tokenRepo.Save(ctx, token); err != nil {
		return "", err
	}
	return token.Token, nil
}
