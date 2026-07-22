# Phase 3 Backend Design

## Overview

Go REST API using Fuego, GORM + PostgreSQL, NocoDB as admin UI. Bun serves SvelteKit in SSR mode. Two admin roles: service_admin (platform) and wedding_admin (per wedding). Full multi-tenant isolation via `wedding_id`. All IDs base64-encoded in API responses.

## Project Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Entry, bootstrap, route registration
├── internal/
│   ├── bootstrap/
│   │   └── bootstrap.go         # DB init, service/handler wiring
│   ├── config/
│   │   ├── config.go            # Env vars, DB config
│   │   └── server.go            # Fuego server factory
│   ├── handlers/
│   │   ├── auth.go              # Login, refresh, logout
│   │   ├── guest.go             # CRUD + check-in + seat assignment
│   │   ├── table.go             # CRUD per wedding
│   │   ├── wedding.go           # CRUD + list
│   │   ├── admin.go             # Service admin CRUD
│   │   └── register.go          # Route registration helpers
│   ├── middleware/
│   │   ├── auth.go              # JWT validation, context injection
│   │   ├── wedding_scope.go     # Tenant isolation check
│   │   └── cors.go              # CORS for Bun dev server
│   ├── models/
│   │   ├── admin_user.go        # Admin user (service + wedding admin)
│   │   ├── wedding_event.go     # Wedding event (tenant root)
│   │   ├── guest_record.go      # Guest with seat range
│   │   ├── banquet_table.go     # Banquet table layout
│   │   └── refresh_token.go     # Refresh token storage
│   ├── repository/
│   │   ├── admin_repo.go        # GORM queries for admin
│   │   ├── wedding_repo.go      # GORM queries for wedding
│   │   ├── guest_repo.go        # GORM queries for guest
│   │   ├── table_repo.go        # GORM queries for table
│   │   └── token_repo.go        # GORM queries for refresh tokens
│   ├── services/
│   │   ├── auth_service.go      # JWT generation, password hashing
│   │   ├── guest_service.go     # Business logic, seat validation
│   │   ├── table_service.go     # Table CRUD, occupancy calc
│   │   └── wedding_service.go   # Wedding CRUD
│   └── utils/
│       └── encoding.go          # Base64 ID encode/decode
├── go.mod
├── go.sum
└── .env.example
```

## Architecture Pattern

Layered: **Handler → Service → Repository**

- **Handler** — HTTP in/out, Fuego request/response types. No business logic.
- **Service** — business rules, orchestration. No HTTP or DB specifics.
- **Repository** — GORM queries only. No business logic.

Follows same pattern as `actual_helper` repo, replacing "providers" with "repository" for GORM access.

## GORM Models

### WeddingEvent (tenant root)

```go
type WeddingEvent struct {
    ID        uint      `gorm:"primaryKey" json:"-"`
    Name      string    `gorm:"size:255;not null" json:"n"`
    Date      time.Time `json:"d"`
    CreatedAt time.Time `json:"c"`
    UpdatedAt time.Time `json:"u"`
}
```

### AdminUser

```go
type AdminUser struct {
    ID        uint      `gorm:"primaryKey" json:"-"`
    WeddingID *uint     `gorm:"index" json:"-"`     // nil for service_admin
    Email     string    `gorm:"size:255;not null" json:"e"`
    Password  string    `gorm:"size:255;not null" json:"-"`
    Name      string    `gorm:"size:255" json:"n"`
    Role      string    `gorm:"size:20;not null" json:"rl"` // "service_admin" or "wedding_admin"
    CreatedAt time.Time `json:"c"`
    UpdatedAt time.Time `json:"u"`
}
```

- `WeddingID` nullable — nil for service admins
- `Role` checked in middleware
- Service admin sees all weddings, wedding admin scoped to own

### BanquetTable

```go
type BanquetTable struct {
    ID        uint      `gorm:"primaryKey" json:"-"`
    WeddingID uint      `gorm:"index;not null" json:"-"`
    Name      string    `gorm:"size:100;not null" json:"n"`
    Capacity  int       `gorm:"not null" json:"cap"`
    X         float64   `json:"x"`
    Y         float64   `json:"y"`
    IsVip     bool      `json:"v"`
    Zone      string    `gorm:"size:20" json:"z"`
    CreatedAt time.Time `json:"c"`
    UpdatedAt time.Time `json:"u"`
}
```

### GuestRecord

```go
type GuestRecord struct {
    ID          uint       `gorm:"primaryKey" json:"-"`
    WeddingID   uint       `gorm:"index;not null" json:"-"`
    Name        string     `gorm:"size:255;not null" json:"n"`
    Phone       string     `gorm:"size:50" json:"p"`
    Email       string     `gorm:"size:255" json:"e"`
    Pax         int        `gorm:"not null;default:1" json:"x"`
    TableID     *uint      `gorm:"index" json:"-"`
    SeatNum     *int       json:"-"`
    RSVP        string     `gorm:"size:20;default:no_response" json:"r"`
    CheckedInAt *time.Time `json:"cia"`
    Notes       string     `gorm:"type:text" json:"nt"`
    Dietary     []string   `gorm:"type:text[]" json:"d"`
    IsVip       bool       `json:"v"`
    AngbaoAmt   *int       `json:"a"`
    GiftItem    *string    `gorm:"size:255" json:"g"`
    CreatedAt   time.Time  `json:"c"`
    UpdatedAt   time.Time  `json:"u"`
}
```

- Seat model: `SeatNum` + `Pax` → consecutive seats from `SeatNum` to `SeatNum+Pax-1`
- `CheckedInAt != nil` means checked in (no separate bool)
- `TableID` + `SeatNum` nullable for unassigned guests

### RefreshToken

```go
type RefreshToken struct {
    ID        uint      `gorm:"primaryKey"`
    AdminID   uint      `gorm:"index;not null"`
    Token     string    `gorm:"size:255;uniqueIndex;not null"`
    ExpiresAt time.Time
    CreatedAt time.Time
}
```

## API Routes

### Auth (shared)

```
POST   /api/v1/auth/login              # → { accessToken, refreshToken }
POST   /api/v1/auth/refresh             # → { accessToken }
POST   /api/v1/auth/logout
```

### Service Admin only

```
GET    /api/v1/admins                   # list all admins
POST   /api/v1/admins                   # create admin (assign to wedding)
PUT    /api/v1/admins/:id               # update admin
DELETE /api/v1/admins/:id               # delete admin

GET    /api/v1/weddings                 # list all weddings
POST   /api/v1/weddings                 # create wedding
GET    /api/v1/weddings/:id             # get wedding
PUT    /api/v1/weddings/:id             # update wedding
DELETE /api/v1/weddings/:id             # delete wedding
```

### Wedding Admin (scoped to own wedding)

```
GET    /api/v1/weddings/:wid/tables
POST   /api/v1/weddings/:wid/tables
PUT    /api/v1/weddings/:wid/tables/:id
DELETE /api/v1/weddings/:wid/tables/:id

GET    /api/v1/weddings/:wid/guests
POST   /api/v1/weddings/:wid/guests
GET    /api/v1/weddings/:wid/guests/:id
PUT    /api/v1/weddings/:wid/guests/:id
DELETE /api/v1/weddings/:wid/guests/:id
POST   /api/v1/weddings/:wid/guests/:id/checkin
POST   /api/v1/weddings/:wid/guests/:id/checkout
GET    /api/v1/weddings/:wid/guests/search?q=
GET    /api/v1/weddings/:wid/occupancy
```

## Base64 ID Encoding

All API responses encode `uint` IDs as base64:

```go
func EncodeID(id uint) string {
    return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(id), 10)))
}

func DecodeID(encoded string) (uint, error) {
    b, err := base64.RawURLEncoding.DecodeString(encoded)
    if err != nil {
        return 0, err
    }
    id, err := strconv.ParseUint(string(b), 10, 64)
    if err != nil {
        return 0, err
    }
    return uint(id), nil
}
```

- API response: `"id": "NDI="` (base64 of `"42"`)
- URL params `:id` and `:wid` decoded in middleware
- Frontend treats IDs as opaque strings

## Auth Flow

### Login

```
POST /auth/login { email, password }
→ validate against AdminUser table (bcrypt)
→ generate JWT: { sub: adminId, wid: weddingId, role: role, exp: 15min }
→ generate refresh token (stored in DB, 7 day expiry)
→ return { accessToken, refreshToken }
```

### Refresh

```
POST /auth/refresh { refreshToken }
→ validate refresh token exists + not expired
→ regenerate accessToken with same claims
→ return { accessToken }
```

### Logout

```
POST /auth/logout { refreshToken }
→ delete refresh token from DB
```

### JWT Config

- Signing: HS256 with `JWT_SECRET` env var
- Access token expiry: 15 minutes
- Refresh token expiry: 7 days
- Password hashing: bcrypt via `golang.org/x/crypto/bcrypt`

## Multi-Tenant Middleware

### Auth Middleware

```go
func AuthMiddleware(secret string) func(fuego.Handler) fuego.Handler {
    return func(next fuego.Handler) fuego.Handler {
        return func(c *fuego.ContextWithBody[any]) (any, error) {
            token := extractBearer(c.Request())
            claims, err := validateJWT(token, secret)
            if err != nil {
                return nil, fuego.UnauthorizedError{Title: "Invalid token"}
            }
            c.Set("adminId", claims.AdminID)
            c.Set("weddingId", claims.WeddingID)
            c.Set("role", claims.Role)
            return next(c)
        }
    }
}
```

### Wedding Scope Middleware

```go
func WeddingScopeMiddleware(next fuego.Handler) fuego.Handler {
    return func(c *fuego.ContextWithBody[any]) (any, error) {
        role := c.Get("role").(string)
        if role == "service_admin" {
            return next(c)
        }
        jwtWid := c.Get("weddingId").(uint)
        urlWid, _ := utils.DecodeID(c.PathParam("wid"))
        if jwtWid != urlWid {
            return nil, fuego.ForbiddenError{Title: "Access denied"}
        }
        return next(c)
    }
}
```

- `service_admin` — bypasses scope check, access all weddings
- `wedding_admin` — JWT `wid` must match URL `:wid`

## Frontend Integration

### Runtime

- Bun serves SvelteKit in SSR mode (re-enable `ssr = true`)
- Go API on port 8080, Bun on port 5173
- Frontend calls Go API via `fetch("http://localhost:8080/api/v1/...")`

### Auth Store

```ts
let accessToken = $state<string | null>(null);
let refreshToken = $state<string | null>(null);
let role = $state<string | null>(null);
```

### API Client

```ts
async function apiFetch(path: string, opts: RequestInit = {}) {
    const { accessToken } = getAuth();
    const res = await fetch(`http://localhost:8080${path}`, {
        ...opts,
        headers: {
            ...opts.headers,
            Authorization: `Bearer ${accessToken}`,
        },
    });
    if (res.status === 401) {
        // try refresh, retry
    }
    return res;
}
```

### Table Definitions

- Remove localStorage logic from `constants/index.ts`
- Fetch from API: `GET /weddings/:wid/tables`
- Store in Svelte context, not localStorage

## Database

- PostgreSQL via GORM
- NocoDB connects to same DB as admin UI
- GORM auto-migrates tables on startup
- `wedding_id` index on all resource tables for tenant isolation

## Environment Variables

```
DATABASE_URL=postgresql://user:pass@localhost:5432/weddingdb
JWT_SECRET=your-secret-key
PORT=8080
NOCODB_URL=http://localhost:8081
```
