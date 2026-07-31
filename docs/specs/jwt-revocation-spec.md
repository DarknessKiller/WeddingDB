# JWT Revocation via Redis — Implementation Spec

## Overview

Add per-user token versioning + per-token blacklisting to invalidate JWTs without waiting for expiry.

**Strategy:** Embed `tv` (token_version) in every access JWT. Store current `tv` in Redis per user (lazy init: default 0). On every request, middleware compares JWT `tv` against Redis `tv`. On logout, blacklist the specific access token's JTI in Redis with TTL = remaining token life.

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

- Redis key: `"user:" + userID.String() + ":tv"`
- `GET` the key. If key doesn't exist (redis.Nil), return `0, nil`.
- Parse value as int. Return it.
- On error, return `0, err` (fail-closed: caller rejects the request).

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

- Call `s.tokenRepo.DeleteByAdminID(ctx, userID)` to nuke all refresh tokens in Postgres first (closes TOCTOU race).
- Redis key: `"user:" + userID.String() + ":tv"`
- `INCR` the key (creates with value 1 if absent — satisfies lazy init).
- Return first error encountered.

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
func (r *TokenRepo) DeleteByAdminID(ctx context.Context, adminID uuid.UUID) error
```

- `WHERE admin_id = ?`, delete all matching `RefreshToken` rows.
- Add import for `"github.com/google/uuid"` and `"context"`.

---

## Files NOT modified

- `backend/internal/middleware/nonce.go` — reference only, no changes.
- `backend/internal/handlers/helpers.go` — no changes needed.
- `backend/internal/models/admin_user.go` — no schema change (token_version lives in Redis only).
- `backend/internal/models/refresh_token.go` — no schema change.

---

## Redis Key Summary

| Key pattern | Type | TTL | Purpose |
|---|---|---|---|
| `blacklist:jti:{jti}` | string `"1"` | Remaining token lifetime | Per-token blacklist (logout) |
| `user:{id}:tv` | string (integer) | None (persists) | Per-user token version (admin revoke) |

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
      → tokenRepo.DeleteByAdminID(ctx, userID)  [Postgres, nukes all refresh tokens first]
      → INCR user:{id}:tv  [Redis, creates if absent]
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
      → GET user:{id}:tv → parse int (default 0)
      → if currentTV > claims.TokenVersion → reject "token version is stale"
  → set context values → next handler
```

---

## Edge Cases & Failure Modes

1. **Redis down during middleware check:** `GetTokenVersion` returns `0, err`. `IsTokenBlacklisted` returns `false, err`. Fail-closed: reject request with "service temporarily unavailable".

2. **Redis down during blacklist write (logout):** `BlacklistAccessToken` returns error. Logout still proceeds (refresh token deleted from Postgres). Access token lives until 15min expiry. Acceptable tradeoff.

3. **Redis down during admin revoke:** `INCR` fails. RevokeUserTokens returns error. Handler returns 500. Admin retries.

4. **Token already expired at logout time:** `BlacklistAccessToken` checks `ttl <= 0`, returns nil. No-op. Fine.

5. **First-ever request for a user (no tv in Redis):** `GetTokenVersion` returns `0`. JWT with `tv=0` passes. Correct — lazy init means no write until first revoke.

6. **Race between revoke and in-flight requests:** Requests already past middleware are unaffected. Requests hitting middleware during revoke will see the INCR on next Redis read (sub-ms propagation in single Redis instance).
