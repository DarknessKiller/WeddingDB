package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccessClaims struct {
	AdminID      uuid.UUID  `json:"sub"`
	WeddingID    *uuid.UUID `json:"wid,omitempty"`
	Role         string     `json:"role"`
	TokenVersion int        `json:"tv"`
	// ForcePasswordChange limits the token to the change-password endpoint.
	ForcePasswordChange bool `json:"pwc,omitzero"`
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

// compareHash is bcrypt.CompareHashAndPassword; tests swap it to count calls.
var compareHash = bcrypt.CompareHashAndPassword

// dummyHash lets unknown-email logins spend the same bcrypt time as real ones.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("dummy bcrypt hash: %v", err))
	}
	return h
})

func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	admin, err := s.adminRepo.FindByEmail(ctx, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		compareHash(dummyHash(), []byte(password)) //nolint:errcheck // timing equalizer only; result is always a mismatch
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("find admin: %w", err)
	}
	if err := compareHash([]byte(admin.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	tv, err := s.GetTokenVersion(ctx, admin.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to read token version: %w", err)
	}
	accessToken, err := s.signAccessToken(admin, nil, tv)
	if err != nil {
		return nil, err
	}
	refresh, err := s.buildRefreshToken(admin.ID, nil)
	if err != nil {
		return nil, err
	}
	// A revoke (password change, demotion) that landed after admin was read
	// must not leave this login holding live refresh token.
	if err := s.tokenRepo.SaveAtVersion(ctx, refresh, tv); err != nil {
		if errors.Is(err, repository.ErrTokenVersionStale) {
			// The cache may also lag a revoke whose Redis write failed: lift it
			// so next attempt mints at current version.
			dbTV, err := s.adminRepo.TokenVersion(ctx, admin.ID)
			if err == nil {
				err = raiseTV.Run(ctx, s.redisClient, []string{tvKey(admin.ID)}, dbTV).Err()
			}
			if err != nil {
				slog.Warn("raise cached token version after stale login", "user", admin.ID, "err", err)
			}
			return nil, ErrSessionsChanged
		}
		return nil, err
	}
	return s.sessionResult(ctx, admin, accessToken, refresh.Token)
}

// sessionResult assembles the token payload shared by Login and Refresh.
func (s *AuthService) sessionResult(ctx context.Context, admin *models.AdminUser, accessToken, refreshToken string) (*LoginResult, error) {
	var (
		weddings []models.WeddingEvent
		err      error
	)
	if admin.Role == "admin" {
		weddings, err = s.weddingRepo.List(ctx)
	} else {
		weddings, err = s.adminRepo.GetUserWeddings(ctx, admin.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("load weddings for admin %s: %w", admin.ID, err)
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
// refreshToken identifies the caller's device session; only that session's
// scope changes.
func (s *AuthService) SelectWedding(ctx context.Context, adminID, weddingID uuid.UUID, refreshToken string) (string, error) {
	admin, err := s.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrAdminNotFound
		}
		return "", fmt.Errorf("select wedding: find admin: %w", err)
	}
	// Verify access
	if admin.Role != "admin" {
		hasAccess, err := s.adminRepo.HasWeddingAccess(ctx, adminID, weddingID)
		if err != nil {
			return "", fmt.Errorf("select wedding: check access: %w", err)
		}
		if !hasAccess {
			return "", ErrNoWeddingAccess
		}
		// Persist the wedding scope onto the caller's refresh token so that
		// Refresh re-mints a scoped access token. Without this, the first
		// refresh after SelectWedding drops the wedding and the user gets 403
		// until they log in again. Admins keep a nil scope (all weddings).
		found, err := s.tokenRepo.UpdateWeddingScope(ctx, adminID, refreshToken, weddingID)
		if err != nil {
			return "", fmt.Errorf("failed to persist wedding scope: %w", err)
		}
		if !found {
			return "", ErrSessionNotFound
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
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	if token.RotatedAt != nil {
		return s.resumeRotation(ctx, token, refreshTokenStr)
	}

	admin, err := s.adminRepo.FindByID(ctx, token.AdminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find admin: %w", err)
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
	// Derived, not random: only hashes are stored, so a grace-window retry
	// can recover this same successor from the token it presents.
	replacement.Token = s.successorToken(refreshTokenStr)

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
		return nil, ErrInvalidRefreshToken
	}
	return s.resumeRotation(ctx, current, refreshTokenStr)
}

// successorToken is the refresh token that presented rotates into. It is an
// HMAC under the server secret, so it cannot be computed from a leaked hash.
func (s *AuthService) successorToken(presented string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte("refresh-successor:" + presented))
	return hex.EncodeToString(mac.Sum(nil))
}

// resumeRotation answers a retry of an already-rotated token with the same
// successor. Inside the grace window any retry is answered. Past it, the
// successor is served only while it is unused: that is the same client whose
// rotation response was lost (flaky venue WiFi). Once the successor itself
// was used, the old token is a replay and the whole family is revoked.
// presented is the raw secret of consumed, which derives the successor.
func (s *AuthService) resumeRotation(ctx context.Context, consumed *models.RefreshToken, presented string) (*LoginResult, error) {
	if consumed.RotatedAt == nil || consumed.ReplacedBy == nil {
		return nil, ErrInvalidRefreshToken
	}
	successor := s.successorToken(presented)
	if *consumed.ReplacedBy != repository.HashToken(successor) {
		// Rotated into a token this server did not derive (another secret).
		return nil, ErrInvalidRefreshToken
	}
	replacement, err := s.tokenRepo.FindByToken(ctx, successor)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find successor refresh token: %w", err)
	}
	pastWindow := time.Since(*consumed.RotatedAt) > refreshGraceWindow
	successorUsed := err != nil || replacement.RotatedAt != nil
	if pastWindow && successorUsed {
		// The legitimate holder already moved on, so this token leaked. Kill
		// the whole family instead of serving it to a possible attacker.
		if err := s.RevokeUserTokens(ctx, consumed.AdminID); err != nil {
			return nil, fmt.Errorf("revoke replayed refresh token family: %w", err)
		}
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	// Answered, but logged so a repeated pattern is visible.
	slog.Warn("rotated refresh token reused", "user", consumed.AdminID, "rotatedAt", consumed.RotatedAt, "pastGraceWindow", pastWindow)
	admin, err := s.adminRepo.FindByID(ctx, replacement.AdminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find admin: %w", err)
	}
	weddingID, err := s.activeWeddingScope(ctx, admin.ID, replacement.WeddingID)
	if err != nil {
		return nil, err
	}
	accessToken, err := s.generateAccessToken(ctx, admin, weddingID)
	if err != nil {
		return nil, err
	}
	// A revoke deletes refresh tokens before bumping the token version. If it
	// ran between the lookup above and the mint, the successor is gone now and
	// the minted access token must not be handed out.
	if _, err := s.tokenRepo.FindByToken(ctx, successor); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("recheck successor refresh token: %w", err)
	}
	return s.sessionResult(ctx, admin, accessToken, successor)
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

// CheckAuthRate caps every auth endpoint (login/refresh/register) at 25
// requests per second per client IP. It fails open: a Redis outage must not
// lock users out, and sustained abuse through an outage is itself visible.
func (s *AuthService) CheckAuthRate(ctx context.Context, ip string) error {
	key := fmt.Sprintf("auth:ip:%s:%d", ip, time.Now().Unix())
	return s.rateLimit(ctx, key, 25, time.Second)
}

// CheckLoginAccountRate locks one account after 25 failed logins per 15 min,
// scaled to the 25/s per-IP allowance.
func (s *AuthService) CheckLoginAccountRate(ctx context.Context, email, ip string) error {
	return s.checkRateLimit(ctx, loginFailKey(email, ip), 25, 15*time.Minute)
}

// loginFailKey scopes lockout to email+IP so an attacker cannot lock a victim out from elsewhere.
func loginFailKey(email, ip string) string {
	return "login:acct:" + strings.ToLower(strings.TrimSpace(email)) + ":" + ip
}

// CheckCredentialIPRate caps register attempts at 10 per minute per IP. It
// shares its counter with failed logins (RecordLoginFailure).
func (s *AuthService) CheckCredentialIPRate(ctx context.Context, ip string) error {
	return s.rateLimit(ctx, credIPKey(ip), 10, time.Minute)
}

// CheckLoginIPRate blocks an IP after 10 failed logins per minute without
// counting this attempt: staff behind one venue NAT can all log in at once.
func (s *AuthService) CheckLoginIPRate(ctx context.Context, ip string) error {
	return s.checkRateLimit(ctx, credIPKey(ip), 10, time.Minute)
}

func credIPKey(ip string) string { return "cred:ip:" + ip }

// RecordLoginFailure counts a failed password against the account lockout
// and the per-IP credential limit.
func (s *AuthService) RecordLoginFailure(ctx context.Context, email, ip string) {
	s.recordFailure(ctx, loginFailKey(email, ip), 15*time.Minute)
	s.recordFailure(ctx, credIPKey(ip), time.Minute)
}

func (s *AuthService) checkRateLimit(ctx context.Context, key string, limit int64, window time.Duration) error {
	n, err := s.redisClient.Get(ctx, "ratelimit:"+key).Int64()
	if err != nil { // redis.Nil = under limit; any other error = fail open
		if !errors.Is(err, redis.Nil) {
			slog.Warn("rate limit unavailable, failing open", "key", key, "err", err)
		}
		return nil
	}
	if n >= limit {
		return errors.New("too many attempts, please try again later")
	}
	return nil
}

func (s *AuthService) recordFailure(ctx context.Context, key string, window time.Duration) {
	if _, err := s.incrWindow(ctx, key, window); err != nil {
		slog.Warn("record login failure failed", "key", key, "err", err)
	}
}

// CheckPublicRate caps public kiosk endpoints at 25 req/s per client IP.
func (s *AuthService) CheckPublicRate(ctx context.Context, ip string) error {
	key := fmt.Sprintf("public:ip:%s:%d", ip, time.Now().Unix())
	return s.rateLimit(ctx, key, 25, time.Second)
}

// incrWindow atomically bumps key and sets its TTL on first hit (MULTI/EXEC),
// so a key can never be left without expiry.
func (s *AuthService) incrWindow(ctx context.Context, key string, window time.Duration) (int64, error) {
	var incr *redis.IntCmd
	_, err := s.redisClient.TxPipelined(ctx, func(p redis.Pipeliner) error {
		incr = p.Incr(ctx, "ratelimit:"+key)
		p.ExpireNX(ctx, "ratelimit:"+key, window)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (s *AuthService) rateLimit(ctx context.Context, key string, limit int64, window time.Duration) error {
	n, err := s.incrWindow(ctx, key, window)
	if err != nil {
		slog.Warn("rate limit unavailable, failing open", "key", key, "err", err)
		return nil
	}
	if n > limit {
		return errors.New("too many requests, slow down")
	}
	return nil
}

// ErrRateLimitUnavailable reports that password-guess limit could not be read.
var ErrRateLimitUnavailable = errors.New("sign-in is temporarily unavailable, try again shortly")

// Logout deletes the refresh token and blacklists the access token's JTI.
// Blacklisting is best-effort: a Redis failure does not prevent refresh token deletion.
func (s *AuthService) Logout(ctx context.Context, refreshToken string, accessToken string) error {
	// Blacklist the access token JTI (best-effort — don't block logout on Redis failure)
	if accessToken != "" {
		if claims, err := s.parseTokenClaims(accessToken); err == nil && claims != nil {
			if err := s.BlacklistAccessToken(ctx, claims); err != nil {
				slog.Warn("blacklist access token at logout failed", "user", claims.Subject, "err", err)
			}
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

	if err := s.CheckRevoked(ctx, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// CheckRevoked rejects claims whose JTI is blacklisted or whose token version
// is stale. It fails closed on Redis errors. Long-lived connections such as SSE
// call it periodically, since they outlive the check made at connect time.
func (s *AuthService) CheckRevoked(ctx context.Context, claims *AccessClaims) error {
	// Check blacklist (targeted, time-bounded check — do this first)
	blacklisted, err := s.IsTokenBlacklisted(ctx, claims.ID)
	if err != nil {
		return fmt.Errorf("service temporarily unavailable: %w", err)
	}
	if blacklisted {
		return errors.New("token has been revoked")
	}

	// Check token version (broader sweep)
	storedTV, err := s.GetTokenVersion(ctx, claims.AdminID)
	if err != nil {
		return fmt.Errorf("service temporarily unavailable: %w", err)
	}
	if claims.TokenVersion < storedTV {
		return errors.New("token version is stale")
	}
	return nil
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

// tvKey is the Redis cache key for a user's token version.
func tvKey(userID uuid.UUID) string { return fmt.Sprintf("user:%s:tv", userID.String()) }

// raiseTV sets the cached token version only if it raises it, so a stale
// cache fill racing a revoke can never move the version backwards.
var raiseTV = redis.NewScript(`
local cur = tonumber(redis.call('GET', KEYS[1]))
if cur == nil or cur < tonumber(ARGV[1]) then
	redis.call('SET', KEYS[1], ARGV[1])
end
return 0`)

// GetTokenVersion returns the user's token version. Postgres holds the truth
// and Redis caches it: a cache miss (e.g. Redis restarted without
// persistence) reloads from Postgres, so a revoke is never forgotten. Any
// error, including an unknown user, is returned so callers fail closed.
func (s *AuthService) GetTokenVersion(ctx context.Context, userID uuid.UUID) (int, error) {
	key := tvKey(userID)
	val, err := s.redisClient.Get(ctx, key).Int()
	if err == nil {
		return val, nil
	}
	if !errors.Is(err, redis.Nil) {
		return 0, fmt.Errorf("read cached token version: %w", err)
	}
	tv, err := s.adminRepo.TokenVersion(ctx, userID)
	if err != nil {
		return 0, err
	}
	if err := raiseTV.Run(ctx, s.redisClient, []string{key}, tv).Err(); err != nil {
		return 0, fmt.Errorf("cache token version: %w", err)
	}
	return tv, nil
}

// RevokeUserTokens deletes the user's refresh tokens and bumps their token
// version in Postgres (one transaction), then raises the Redis cache to match.
func (s *AuthService) RevokeUserTokens(ctx context.Context, userID uuid.UUID) error {
	// A cached version ahead of Postgres (set before versions were persisted)
	// must still be exceeded. A cache error leaves the floor at 0: Postgres
	// still bumps, and the cache write below reports the outage.
	floor, err := s.redisClient.Get(ctx, tvKey(userID)).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		slog.Warn("read cached token version before revoke", "user", userID, "err", err)
	}
	if err := s.tokenRepo.DeleteByAdminID(ctx, userID, floor); err != nil {
		return fmt.Errorf("failed to revoke tokens: %w", err)
	}
	tv, err := s.adminRepo.TokenVersion(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to read token version: %w", err)
	}
	// Retry on transient Redis failures — the Postgres bump is already durable.
	var lastErr error
	for range 3 {
		if lastErr = raiseTV.Run(ctx, s.redisClient, []string{tvKey(userID)}, tv).Err(); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to cache token version after retries: %w", lastErr)
}

// accessTokenTTL is how long an access token lives.
const accessTokenTTL = 15 * time.Minute

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
	return s.signAccessToken(admin, weddingID, tv)
}

func (s *AuthService) signAccessToken(admin *models.AdminUser, weddingID *uuid.UUID, tv int) (string, error) {
	now := time.Now()
	expiry := now.Add(accessTokenTTL)
	claims := AccessClaims{
		AdminID:             admin.ID,
		WeddingID:           weddingID,
		Role:                admin.Role,
		TokenVersion:        tv,
		ForcePasswordChange: admin.ForcePasswordChange,
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
