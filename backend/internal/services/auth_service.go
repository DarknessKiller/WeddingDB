package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	}
	return s.generateAccessToken(ctx, admin, &weddingID)
}

// Refresh returns a new LoginResult with fresh tokens.
// oldAccessToken is the previous access token (from Authorization header) — its JTI is blacklisted
// to prevent concurrent use after rotation.
func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string, oldAccessToken string) (*LoginResult, error) {
	token, err := s.tokenRepo.FindByToken(ctx, refreshTokenStr)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	admin, err := s.adminRepo.FindByID(ctx, token.AdminID)
	if err != nil {
		return nil, errors.New("admin not found")
	}
	accessToken, err := s.generateAccessToken(ctx, admin, token.WeddingID)
	if err != nil {
		return nil, err
	}
	newRefreshToken, err := s.generateRefreshToken(ctx, admin.ID, token.WeddingID)
	if err != nil {
		return nil, err
	}
	s.tokenRepo.DeleteByToken(ctx, refreshTokenStr)

	// Blacklist old access token JTI to prevent concurrent use after rotation
	if oldAccessToken != "" {
		if oldClaims, err := s.parseTokenUnsafe(oldAccessToken); err == nil && oldClaims != nil {
			if err := s.BlacklistAccessToken(ctx, oldClaims); err != nil {
				return nil, fmt.Errorf("failed to blacklist old access token: %w", err)
			}
		}
	}

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
		RefreshToken:        newRefreshToken,
		Role:                admin.Role,
		Name:                admin.Name,
		Weddings:            weddings,
		ForcePasswordChange: admin.ForcePasswordChange,
	}, nil
}

// Logout deletes the refresh token and blacklists the access token's JTI.
func (s *AuthService) Logout(ctx context.Context, refreshToken string, accessToken string) error {
	// Blacklist the access token JTI if provided
	if accessToken != "" {
		claims, err := s.parseTokenUnsafe(accessToken)
		if err == nil && claims != nil {
			if err := s.BlacklistAccessToken(ctx, claims); err != nil {
				return fmt.Errorf("failed to blacklist access token: %w", err)
			}
		}
	}
	return s.tokenRepo.DeleteByToken(ctx, refreshToken)
}

// ValidateToken parses and validates a JWT, then checks blacklist and token version.
func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
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
	if err := s.tokenRepo.DeleteByAdminID(userID); err != nil {
		return fmt.Errorf("failed to delete refresh tokens: %w", err)
	}
	// Step 2: Bump token version. Any existing access token with old tv is now rejected by middleware.
	key := fmt.Sprintf("user:%s:tv", userID.String())
	if err := s.redisClient.Incr(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to increment token version: %w", err)
	}
	return nil
}

// parseTokenUnsafe parses a JWT without blacklist/tv checks — used only for extracting claims during logout.
func (s *AuthService) parseTokenUnsafe(tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
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

func (s *AuthService) generateRefreshToken(ctx context.Context, adminID uuid.UUID, weddingID *uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tokenStr := hex.EncodeToString(b)
	token := &models.RefreshToken{
		ID:        uuid.New(),
		AdminID:   adminID,
		WeddingID: weddingID,
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.tokenRepo.Save(ctx, token); err != nil {
		return "", err
	}
	return tokenStr, nil
}
