# Task 7: Auth Service

**Status:** ✅ Done  
**Commit:** `b15d79e`  
**Files:** `backend/internal/services/auth_service.go` (126 lines)

## What was built
Auth service providing:
- `Login` — email/password → JWT access + refresh tokens
- `Refresh` — rotation (old token deleted, new pair issued)
- `Logout` — invalidates refresh token
- `ValidateToken` — parse and verify JWT claims
- 15min access tokens, 7-day refresh tokens, HS256 signing

## Verification
`go build ./internal/services/` — clean, no errors.
