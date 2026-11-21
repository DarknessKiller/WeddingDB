# JWT Revocation via Redis — Implementation Spec

## Overview

Add per-user token versioning + per-token blacklisting to invalidate JWTs without waiting for expiry.

**Strategy:** Embed `tv` (token_version) in every access JWT. The current `tv` lives in Postgres (`admin_users.token_version`, the source of truth) and is cached in Redis per user. On every request, middleware compares JWT `tv` against the cached `tv`, reloading it from Postgres on a cache miss, so a Redis restart without persistence cannot resurrect revoked tokens. On logout, blacklist the specific access token's JTI in Redis with TTL = remaining token life.

---

## File 1: `backend/internal/services/auth_service.go`

### 1a. Import changes

Add:
```go
"github.com/redis/go-redis/v9"
```

### 1b. Modify `AccessClaims` struct

**Add** `TokenVersion` field:

```go
type AccessClaims struct {
    AdminID      uuid.UUID  `json:"sub"`
    WeddingID    *uuid.UUID `json:"wid,omitempty"`
    Role         string     `json:"role"`
    TokenVersion int        `json:"tv"`
    jwt.RegisteredClaims
}
```

### 1c. Modify `AuthService` struct

**Add** `redisClient *redis.Client` field:

```go
type AuthService struct {
    adminRepo   *repository.AdminRepo
    weddingRepo *repository.WeddingRepo
    tokenRepo   *repository.TokenRepo
    secret      []byte
    redisClient *redis.Client
}
```

### 1d. Modify `NewAuthService` signature

**Before:**
```go
func NewAuthService(adminRepo *repository.AdminRepo, weddingRepo *repository.WeddingRepo, tokenRepo *repository.TokenRepo, secret string) *AuthService
```

**After:**
```go
func NewAuthService(adminRepo *repository.AdminRepo, weddingRepo *repository.WeddingRepo, tokenRepo *repository.TokenRepo, secret string, redisClient *redis.Client) *AuthService
```

Store `redisClient` in struct.

### 1e. Add `GetTokenVersion` method

```go
func (s *AuthService) GetTokenVersion(ctx context.Context, userID uuid.UUID) (int, error)
```

- Redis key: `"user:" + userID.String() + ":tv"` (cache).
- `GET` the key. On a hit, return it.
- On a miss (redis.Nil), load `token_version` from Postgres (`AdminRepo.TokenVersion`), write it back to the cache with a raise-only Lua script (`SET` only if absent or lower, so a stale fill racing a revoke never lowers it), and return it.
- On any Redis or Postgres error — including an unknown (deleted) user — return `0, err` (fail-closed: caller rejects the request). A miss never means "never revoked".

### 1f. Add `BlacklistAccessToken` method

```go
func (s *AuthService) BlacklistAccessToken(ctx context.Context, claims *AccessClaims) error
```

- Compute `ttl = time.Until(claims.ExpiresAt.Time)`. If `ttl <= 0`, return nil (already expired).
- `SET "blacklist:jti:" + claims.ID` = `"1"` with TTL.
- Return error from Redis.

### 1g. Add `IsTokenBlacklisted` method

```go
func (s *AuthService) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
```

- Redis key: `"blacklist:jti:" + jti`
- `EXISTS` the key. Return `exists > 0, nil`.
- On Redis error, return `false, err` (fail-closed: caller rejects the request).

### 1h. Add `RevokeUserTokens` method

```go
func (s *AuthService) RevokeUserTokens(ctx context.Context, userID uuid.UUID) error
```

- Read the cached version as a floor (best-effort; 0 on error). It covers versions that lived only in Redis before Postgres stored them.
- Call `s.tokenRepo.DeleteByAdminID(ctx, userID, floor)`: in one transaction holding the admin row lock, delete all refresh tokens and set `token_version = max(token_version, floor) + 1`. Postgres is written first and is durable.
- Read the new version back and raise the Redis cache to it with the raise-only script (3 attempts).
- Return first error encountered. A failed cache write returns an error even though the Postgres bump stuck.

### 1i. Modify `generateAccessToken` signature

**Before:**
```go
func (s *AuthService) generateAccessToken(admin *models.AdminUser, weddingID *uuid.UUID) (string, error)
```

**After:**
```go
func (s *AuthService) generateAccessToken(ctx context.Context, admin *models.AdminUser, weddingID *uuid.UUID) (string, error)
```

Inside, call `s.GetTokenVersion(ctx, admin.ID)` and set `TokenVersion: tv` in claims struct.

### 1j. Update all callers of `generateAccessToken`

Three call sites — `Login`, `SelectWedding`, `Refresh` — all already have `ctx context.Context`. Change each call from:
```go
s.generateAccessToken(admin, weddingID)
```
to:
```go
s.generateAccessToken(ctx, admin, weddingID)
```

### 1j2. Modify `Refresh` signature

**Before:**
```go
func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string) (*LoginResult, error)
```

**After:**
```go
func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string, oldAccessToken string) (*LoginResult, error)
```

- Before creating new tokens, blacklist old access token's JTI if `oldAccessToken != ""`.
- Parse old token via `parseTokenClaims`, call `BlacklistAccessToken`.
- If blacklisting fails, return error immediately — no token state has been mutated.

### 1k. Modify `ValidateToken` signature

**Before:**
```go
func (s *AuthService) ValidateToken(tokenStr string) (*AccessClaims, error)
```

**After:**
```go
func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (*AccessClaims, error)
```

After parsing + validating the JWT, **add two checks**:

1. **Blacklist check:** `s.IsTokenBlacklisted(ctx, claims.ID)` → if true, return error `"token has been revoked"`.
2. **Token version check:** `s.GetTokenVersion(ctx, claims.AdminID)` → if `claims.TokenVersion < storedTV`, return error `"token version is stale"`.

### 1l. Add `parseTokenClaims` helper

```go
func (s *AuthService) parseTokenClaims(tokenStr string) (*AccessClaims, error)
```

- Parses JWT without blacklist/tv checks — used only for extracting claims during logout and refresh token rotation.
- Same `jwt.ParseWithClaims` logic as `ValidateToken` but returns claims without Redis checks.

### 1m. Modify `Logout` signature

**Before:**
```go
func (s *AuthService) Logout(refreshToken string) error
```

**After:**
```go
func (s *AuthService) Logout(ctx context.Context, refreshToken string, accessToken string) error
```

- Call `s.BlacklistAccessToken(ctx, accessToken)`. Best-effort: ignore error (don't block logout on Redis failure).
- Call `s.tokenRepo.DeleteByToken(refreshToken)`.
- Return the error.

---

## File 2: `backend/internal/middleware/auth.go`

### 2a. Modify `AuthMiddleware` call to `ValidateToken`

**Before:**
```go
claims, err := authService.ValidateToken(token)
```

**After:**
```go
claims, err := authService.ValidateToken(r.Context(), token)
```

No other changes needed. The middleware already has `r.Context()` and passes it to the next handler. The `extractBearer` helper stays as-is.

---

## File 3: `backend/internal/handlers/auth.go`

### 3b. Modify `Refresh` handler

- Extract old access token from Authorization header via `extractBearer(c.Request())`.
- Pass it to `h.authService.Refresh(c.Context(), body.RefreshToken, oldAccessToken)`.
- The service blacklists the old token's JTI after rotation.

### 3c. Add `extractBearer` helper

```go
func extractBearer(r *http.Request) string
```

- Gets `Authorization` header, strips `"Bearer "` prefix via `strings.CutPrefix`.
- Returns empty string if not present or not Bearer scheme.
- Add imports for `"net/http"` and `"strings"`.

### 3a. Modify `Logout` handler

**Before:** Takes `RefreshRequest` body, calls `h.authService.Logout(body.RefreshToken)`.

**After:**

```go
func (h *AuthHandler) Logout(c fuego.ContextWithBody[RefreshRequest]) (any, error)
```

- Extract access token from Authorization header: `c.Request().Header.Get("Authorization")`, strip `"Bearer "` prefix.
- Call `h.authService.Logout(c.Context(), body.RefreshToken, accessToken)`.
- Rest unchanged.

Need to add import for `"strings"` if not already present (it's not currently imported in auth.go).

---

## File 4: `backend/internal/handlers/admin.go`

### 4a. Import changes

Add:
```go
"weddingdb/internal/services"
```

### 4b. Modify `AdminHandler` struct

**Before:**
```go
type AdminHandler struct{ adminRepo *repository.AdminRepo }
```

**After:**
```go
type AdminHandler struct {
    adminRepo   *repository.AdminRepo
    authService *services.AuthService
}
```

### 4c. Modify `NewAdminHandler` signature

**Before:**
```go
func NewAdminHandler(adminRepo *repository.AdminRepo) *AdminHandler
```

**After:**
```go
func NewAdminHandler(adminRepo *repository.AdminRepo, authService *services.AuthService) *AdminHandler
```

Store `authService` in struct.

### 4d. Add `RevokeUser` method

```go
func (h *AdminHandler) RevokeUser(c fuego.ContextNoBody) (any, error)
```

- Call `requireAdmin(c.Context())` — admin-only guard.
- Decode path param `"id"` via `DecodeID(c.PathParam("id"))`.
- Prevent self-revoke: if `id == callerID`, return `fuego.BadRequestError`.
- Verify user exists: `h.adminRepo.FindByID(id)`, return `fuego.NotFoundError` if not found.
- Call `h.authService.RevokeUserTokens(c.Context(), id)`.
- On error, return `fuego.InternalServerError`.
- Return `map[string]any{"message": "User tokens revoked"}`.

---

## File 5: `backend/internal/handlers/register.go`

### 5a. Modify `NewAdminHandler` call

**Before:**
```go
adminHandler := NewAdminHandler(adminRepo)
```

**After:**
```go
adminHandler := NewAdminHandler(adminRepo, authService)
```

### 5b. Add revoke route

In the auth-protected section, after the existing user CRUD routes, add:

```go
fuego.Put(api, "/users/{id}/revoke", adminHandler.RevokeUser)
```

---

## File 6: `backend/internal/bootstrap/bootstrap.go`

### 6a. Modify `NewAuthService` call

**Before:**
```go
authService := services.NewAuthService(adminRepo, weddingRepo, tokenRepo, env.JWTSecret)
```

**After:**
```go
authService := services.NewAuthService(adminRepo, weddingRepo, tokenRepo, env.JWTSecret, rdb)
```

`rdb` is already declared and initialized earlier in the function. No other bootstrap changes needed.

---

## File 7: `backend/internal/repository/token_repo.go`

### 7a. Add `DeleteByAdminID` method

```go
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID, minVersion int) error
```

- In one transaction: lock the admin row (`FOR UPDATE`), delete all `RefreshToken` rows `WHERE admin_id = ?`, and set `admin_users.token_version = max(token_version, minVersion) + 1`.
- Add import for `"github.com/google/uuid"` and `"context"`.

---

## Files NOT modified

- `backend/internal/middleware/nonce.go` — reference only, no changes.
- `backend/internal/handlers/helpers.go` — no changes needed.
- `backend/internal/models/admin_user.go` — adds `TokenVersion int` (`token_version`, `not null default 0`), created by the existing `AutoMigrate`.
- `backend/internal/models/refresh_token.go` — no schema change.

---

## Redis Key Summary

| Key pattern | Type | TTL | Purpose |
|---|---|---|---|
| `blacklist:jti:{jti}` | string `"1"` | Remaining token lifetime | Per-token blacklist (logout) |
| `user:{id}:tv` | string (integer) | None | Cache of `admin_users.token_version`; refilled from Postgres on a miss |

---

## Data Flow: Logout

```
Client → POST /api/auth/logout {refreshToken: "..."} + Authorization: Bearer <access>
  → AuthHandler.Logout
    → extract Bearer from header
    → authService.Logout(ctx, refreshToken, accessToken)
      → BlacklistAccessToken(ctx, accessToken)
        → parse claims → extract JTI + expiry
        → SET blacklist:jti:{jti} = "1" EX {remaining seconds}
        → if Redis error → best-effort (ignored, logout continues)
      → tokenRepo.DeleteByToken(refreshToken)  [Postgres]
  → 204
```

## Data Flow: Admin Revoke

```
Client → PUT /api/users/{id}/revoke + Authorization: Bearer <admin-token>
  → AuthMiddleware validates admin token (blacklist + tv checks pass)
  → AdminHandler.RevokeUser
    → requireAdmin(ctx) guard
    → DecodeID, prevent self-revoke, verify user exists
    → authService.RevokeUserTokens(ctx, userID)
      → GET user:{id}:tv as floor  [Redis, best-effort]
      → tokenRepo.DeleteByAdminID(ctx, userID, floor)  [Postgres tx: lock admin row, delete refresh tokens, token_version = max(token_version, floor)+1]
      → raise user:{id}:tv to the new token_version  [Redis cache]
  → 200 {"message": "User tokens revoked"}
```

## Data Flow: Middleware Check (every auth-protected request)

```
Request → AuthMiddleware
  → extractBearer(r) → token string
  → authService.ValidateToken(ctx, token)
    → jwt.ParseWithClaims → claims (includes tv, jti)
    → IsTokenBlacklisted(ctx, claims.ID)
      → EXISTS blacklist:jti:{jti}
      → if exists → reject "token has been revoked"
    → GetTokenVersion(ctx, claims.AdminID)
      → GET user:{id}:tv → on miss load admin_users.token_version and refill cache
      → if currentTV > claims.TokenVersion → reject "token version is stale"
  → set context values → next handler
```

---

## Edge Cases & Failure Modes

1. **Redis down during middleware check:** `GetTokenVersion` returns `0, err`. `IsTokenBlacklisted` returns `false, err`. Fail-closed: reject request with "service temporarily unavailable".

2. **Redis down during blacklist write (logout):** `BlacklistAccessToken` returns error. Logout still proceeds (refresh token deleted from Postgres). Access token lives until 15min expiry. Acceptable tradeoff.

3. **Redis down during admin revoke:** Postgres is bumped, the cache write fails. RevokeUserTokens returns error. Handler returns 500 and leaves role/delete unchanged. Admin retries once Redis is back.

4. **Token already expired at logout time:** `BlacklistAccessToken` checks `ttl <= 0`, returns nil. No-op. Fine.

5. **No tv in Redis (first request, or Redis lost its data):** `GetTokenVersion` loads `token_version` from Postgres and refills the cache. A revoked token stays rejected. A deleted user has no row, so their tokens are rejected.

6. **Migration from Redis-only versions:** `AutoMigrate` adds `token_version` as 0 for existing users. The Redis cache still holds older bumps and keeps being read. The next revoke uses it as a floor, so Postgres overtakes it. Only if Redis loses data before that revoke do tokens revoked before the deploy work again, and only until they expire (≤15 min).

7. **Race between revoke and in-flight requests:** Requests already past middleware are unaffected. Requests hitting middleware during revoke see the raised cache value on the next Redis read (sub-ms propagation in single Redis instance).

---

## Refresh Rotation & Grace Window

`Refresh` mints the access token and the successor refresh token **before**
consuming the presented one, then claims and replaces it in a single Postgres
transaction (`TokenRepo.Rotate`). The claim is a conditional update on
`rotated_at IS NULL`, so exactly one concurrent caller wins; the loser gets
`repository.ErrTokenClaimed` and is served the successor instead of a 401.

A consumed token stays in the table with `rotated_at` + `replaced_by` set. For
`refreshGraceWindow` (60s) after the rotation, presenting it again returns the
same successor — a response lost in transit, or a second tab replaying the
token it still holds, no longer ends the session. Each such reuse is logged, so
a repeated pattern is visible even though it cannot be told apart from an
honest retry. Past the window the same successor is still returned while it
has never been used itself: that is a client whose rotation response was lost
(venue WiFi dropped) and which retries minutes later. Once the successor has
been used, or is gone, a replay of the old token is treated as stolen:
`RevokeUserTokens` deletes the family's refresh tokens and bumps the token
version, killing the attacker's access tokens too. A replay that mints an
access token re-reads the successor afterwards and fails if a concurrent
revoke deleted it.

Consumed rows are reclaimed by `TokenRepo.DeleteExpired` once `expires_at`
passes. Migration is two nullable columns via the existing `AutoMigrate` in
`bootstrap.go`; no backfill.
