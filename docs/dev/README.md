# WeddingDB developer documentation

Chinese wedding seating & reservation management system.

- **Frontend:** SvelteKit SPA (Svelte 5 runes + Tailwind CSS v4 + TypeScript)
- **Backend:** Go REST API (Fuego + GORM + PostgreSQL)
- **Realtime:** Redis (DragonflyDB-compatible) pub/sub + SSE

---

## Project structure

```
├── frontend/       # SvelteKit SPA (SSR disabled, client-only SPA)
├── backend/        # Go REST API (Fuego + GORM + PostgreSQL)
├── docs/dev/       # Developer documentation
└── ui-prototype/   # HTML/CSS/JS design preview (gitignored)
```

## Where features live

`docs/` describes intent and decisions; this table is the map from feature to code. Behaviour questions are answered from the code, not from an ADR's prose.

| Feature | Backend | Frontend |
|---|---|---|
| Guest CRUD, search, pinyin index | `backend/internal/handlers/guest.go`, `backend/internal/models/pinyin.go` | `frontend/src/lib/api/guests.ts`, `frontend/src/lib/utils/guestSearch.ts` |
| Offline queue + newer-time-wins sync | `backend/internal/handlers/guest.go` (`GuestHandler.Sync`), `backend/internal/services/guest_service.go` (`GuestService.Sync`) | `frontend/src/lib/offline/queue.ts`, `frontend/src/routes/[wid]/+layout.svelte` |
| Live updates (SSE) | `backend/internal/services/sse.go`, `backend/internal/handlers/sse.go` | `frontend/src/lib/api/sse.ts`, `frontend/src/lib/stores/guestEvents.ts` |
| Seating, hall map, tables | `backend/internal/handlers/layout.go`, `backend/internal/handlers/table.go` | `frontend/src/lib/components/seating/` |
| Angpao reports, CSV/XLSX export | `backend/internal/services/report_service.go`, `backend/internal/handlers/report.go` | `frontend/src/routes/[wid]/reports/+page.svelte` |
| Auth, JWT refresh rotation, revocation | `backend/internal/services/auth_service.go`, `backend/internal/handlers/auth.go` | `frontend/src/lib/api/client.ts` |
| Guest kiosk (no login) | `backend/internal/handlers/public_guest.go` | `frontend/src/routes/kiosk/[wid]/+page.svelte` |

Docs: `docs/adr/` for decisions, `docs/specs/` for implementation specs, `docs/dev/README.md` (this file) for setup.

Docs are linted against the code: `node scripts/check-docs.mjs` (run in CI) fails on a doc that names a missing path, a missing function, or an ADR `Status:` other than `Proposed` / `Accepted — not implemented` / `Implemented: <paths>`. Tokens for historical mentions of removed code go in `scripts/docs-allow.txt`.

## Tech stack

### Frontend
- **Framework:** SvelteKit (SSR disabled, client-only SPA)
- **UI:** Svelte 5 (runes), Tailwind CSS v4, TypeScript
- **Validation:** Zod v4
- **Hall map:** Konva + svelte-konva (interactive banquet hall editor)
- **Data fetching:** TanStack Query, dayjs, lucide-svelte, CVA + tailwind-merge
- **Fonts:** Inter + Noto Serif SC (Google Fonts)
- **Theme:** Deep Red (#A11217), Gold (#D4AF37), White, Light Beige

### Backend
- **Language:** Go 1.27
- **HTTP:** Fuego
- **ORM:** GORM + PostgreSQL
- **Cache/Pub-Sub:** Redis (nonce/replay prevention, token revocation, SSE cross-instance broadcasting)
- **Auth:** JWT HS256 + bcrypt + refresh token rotation + Redis token blacklisting

### Deployment
- Docker Compose (`docker-compose.yml`; the `redis` service runs DragonflyDB), distroless image, timezone set.
- The Release workflow publishes images to GitHub Container Registry (`ghcr.io/darknesskiller/weddingdb`) on `v*` tags (multi-arch: amd64 + arm64). The compose file pulls `ghcr.io/darknesskiller/weddingdb:latest`, so nothing builds locally.
- CI (`ci.yml`) runs on push and PR to `main`: backend `go test ./...`, frontend `npm run check`, `vitest run`, and `npm run build`.

## Deployment (Docker Compose)

The compose stack runs three services: `app` (the WeddingDB image), `postgres`, and `redis` (DragonflyDB). CI builds and publishes the app image on `v*` tags, so there is no local build step.

### Quick start

```sh
# Set a real secret (required)
export JWT_SECRET=$(openssl rand -hex 32)

docker compose up -d
```

- App: `http://localhost:8080` (serves the SPA + API)
- Postgres: `localhost:5432` (user/pass/db all `weddingdb`, bound to loopback only)
- Redis: internal to the compose network (no host port published)

### File-size limits

Uploads (`POST /api/upload`) have no app-level size cap and no request deadline. Put the size limit in the reverse proxy, and raise the upstream read timeout so large or slow uploads survive:

```nginx
client_max_body_size 50m;            # nginx
proxy_read_timeout 300s;
```

```caddy
reverse_proxy localhost:8080 {
    request_body max_size 50MB        # Caddy
}
```

### First login (bootstrap admin)

On first start with an empty database, the app seeds a default admin:

- **Email:** `admin@weddingdb.local`
- **Password:** whatever you set in `ADMIN_BOOTSTRAP_PASSWORD` (required on first boot; `log.Fatal` if missing)
- The seeded account is flagged `ForcePasswordChange`, so the first login asks for a new password.

The bootstrap only runs when the admin table is empty. After the first admin exists, `ADMIN_BOOTSTRAP_PASSWORD` is ignored.

### Configuration

| Env var (on `app`) | Default | Notes |
|---|---|---|
| `PORT` | `8080` | App listen port |
| `DATABASE_URL` | compose-provided | Points at the `postgres` service; override to use an external DB |
| `REDIS_URL` | `redis://redis:6379` | Compose-provided; override for external Redis |
| `JWT_SECRET` | none | **Required**. Compose fails fast if unset; values shipped in `.env.example` are rejected at boot |
| `REDIS_PASSWORD` | unset (no auth) | Optional. When set, Dragonfly runs with `--requirepass` and the app authenticates via `REDIS_URL` |
| `ADMIN_BOOTSTRAP_PASSWORD` | none | First-boot admin password (required when the database is empty) |
| `TZ` | `Asia/Kuala_Lumpur` | Report timestamps follow this |
| `PUBLIC_URL` | `http://localhost:{PORT}` | Public server URL, used in the OpenAPI spec |
| `CORS_ORIGIN` | `http://localhost:5173` | Allowed origin. Credentials are allowed only for this origin; unset reflects the request origin without credentials |
| `TRUSTED_PROXIES` | unset | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is trusted; unset ignores the header |
| `OTEL_ENABLED` | `false` | Enables OpenTelemetry tracing. See ADR 002 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318/v1/traces` | OTLP HTTP endpoint |
| `OTEL_LOG_SQL` | off | `all` records SQL text on `gorm.query` spans (may contain PII) |
| `OTEL_LOG_BODY` | off | `all` records JSON request/response bodies on request spans (passwords masked) |

### Building locally instead

If you don't want the released image, build from source:

```sh
docker compose build app
# then either run that build:
docker compose up -d
# or swap the compose file to `build: .` and remove `image:`
```

### Data and persistence

- `weddingdb_pgdata` volume: PostgreSQL data. Survives `down`, removed by `down -v`.
- `weddingdb_uploads` volume: uploaded kiosk logos and backgrounds, mounted at `/app/uploads`.
- Backups: `./backup-db.sh` dumps the `postgres` container to `./backups/weddingdb_<timestamp>.sql.gz`. It expects the container to be named `weddingdb-postgres-1` (the default compose naming).
- Restore: `gunzip -c backups/<file>.sql.gz | docker exec -i weddingdb-postgres-1 psql -U weddingdb weddingdb`

### Updating

```sh
docker compose pull app && docker compose up -d
```

New releases are tagged `v*`, and `latest` tracks the newest tag. To pin a version for reproducibility, use `ghcr.io/darknesskiller/weddingdb:v0.1.0`.

---

## Getting started locally

### Frontend

```sh
cd frontend
npm install
npm run dev
```

Server runs at `http://localhost:5173`.

### Backend

```sh
cd backend
# Copy and configure .env
cp .env.example .env
# Edit .env with your database credentials

go run ./cmd/server/
```

Server runs at `http://localhost:8080`.

### Environment variables

Full list: [Configuration](#configuration). Nothing is set twice in this file.

For local runs the values differ from compose: `DATABASE_URL` and `REDIS_URL` point at `localhost`, and `ADMIN_BOOTSTRAP_PASSWORD` is not needed after the first boot.

### Tracing (Jaeger)

Opt-in tracing with a local Jaeger. The compose file ships a `jaeger` service behind the `tracing` profile:

```sh
docker compose --profile tracing up -d jaeger
# then set on the app service:
#   OTEL_ENABLED: "true"
#   OTEL_EXPORTER_OTLP_ENDPOINT: "http://jaeger:4318/v1/traces"
docker compose up -d app
```

Jaeger UI at `http://localhost:16686`. Request spans record the route pattern, the status code, and optionally bodies as span events. Every DB query gets a `gorm.query` span with the rows returned, and with `OTEL_LOG_SQL=all` the SQL text as an event. Bodies mask passwords as `***`. The Badger storage backend keeps traces across Jaeger restarts in the `weddingdb_jqdata` volume.

---

## Pages

| Route | Description |
|-------|-------------|
| `/dashboard` | Stats overview, recent activity, table occupancy |
| `/guests` | Guest list with search, filter, status badges |
| `/seating` | Interactive banquet hall map (most important page) |
| `/search` | Guest search with check-in workflow |
| `/reservation` | Guest registration with seat picker |
| `/tables` | Table overview with capacity bars |
| `/users` | Admin user management |
| `/settings` | App settings (placeholder) |
| `/change-password` | Change password (post-login for forced changes) |
| `/kiosk` | Standalone guest self-service kiosk |
| `/reports` | Report exports (CSV/XLSX) for angpao data |

---

## API endpoints

### Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Login, returns access + refresh tokens |
| POST | `/api/auth/register` | Register new user account |
| POST | `/api/auth/refresh` | Refresh access token |
| POST | `/api/auth/logout` | Revoke refresh + blacklist access token |
| POST | `/api/auth/select-wedding` | Select wedding context |
| POST | `/api/auth/change-password` | Change password |

### Admin and users

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/users` | List admin users |
| POST | `/api/users` | Create admin user |
| DELETE | `/api/users/{id}` | Delete admin user |
| GET | `/api/users/{id}/weddings` | Get user wedding assignments |
| PUT | `/api/users/{id}/weddings` | Assign weddings to user |
| PUT | `/api/users/{id}/role` | Update user role |
| POST | `/api/users/{id}/reset-password` | Reset user password |
| PUT | `/api/users/{id}/revoke` | Revoke all user tokens (admin only) |

### Weddings

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/weddings` | List weddings |
| POST | `/api/weddings` | Create wedding |
| GET | `/api/weddings/{id}` | Get wedding |
| PUT | `/api/weddings/{id}` | Update wedding |
| PUT | `/api/weddings/{id}/kiosk` | Update kiosk settings |
| DELETE | `/api/weddings/{id}` | Delete wedding |
| POST | `/api/upload` | Upload file (multipart, images only; no app-level size cap, enforced by reverse proxy) |

### Tables, guests and seating

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/weddings/{wid}/tables` | List tables |
| POST | `/api/weddings/{wid}/tables` | Create table |
| PUT | `/api/weddings/{wid}/tables/{id}` | Update table |
| DELETE | `/api/weddings/{wid}/tables/{id}` | Delete table |
| GET | `/api/weddings/{wid}/guests` | List guests (cursor pagination: `?cursor=&limit=`, returns `nextCursor`) |
| POST | `/api/weddings/{wid}/guests` | Create guest |
| GET | `/api/weddings/{wid}/guests/{id}` | Get guest |
| PUT | `/api/weddings/{wid}/guests/{id}` | Update guest |
| DELETE | `/api/weddings/{wid}/guests/{id}` | Delete guest |
| POST | `/api/weddings/{wid}/guests/{id}/checkin` | Check in guest (optional body `{"notes": "..."}`; promotes RSVP to confirmed) |
| POST | `/api/weddings/{wid}/guests/{id}/checkout` | Check out guest |
| POST | `/api/weddings/{wid}/guests/{id}/seat` | Assign seat |
| POST | `/api/weddings/{wid}/guests/import` | Bulk import guests (1,000 guests per wedding, counted per person) |
| GET | `/api/weddings/{wid}/guests/search?q=` | Search guests |
| GET | `/api/weddings/{wid}/occupancy` | Table occupancy |
| GET | `/api/weddings/{wid}/layout` | Get hall layout (tables + elements with positions) |
| PATCH | `/api/weddings/{wid}/layout` | Save hall layout (atomic replace) |

Guest limit: 1,000 guests per wedding, counted per person. A guest entry for a couple counts as two.

### Reports and realtime

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/weddings/{wid}/reports/angpao?format=csv\|xlsx` | Export angpao report (CSV or Excel) |
| GET | `/api/weddings/{wid}/events` | SSE stream for real-time guest events (auth via `?token=`) |

### Public endpoints (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/public/weddings/{wid}/guests` | List guests (public view) |
| GET | `/api/public/weddings/{wid}/guests/search?q=` | Search guests (public) |
| GET | `/api/public/weddings/{wid}/tables` | List tables (public) |
| GET | `/api/public/weddings/{wid}/kiosk` | Get kiosk settings |
| GET | `/api/public/weddings/{wid}/layout` | Get hall layout (public view) |

---

## Background dev server

```sh
nohup sh -c 'cd frontend && npm run dev -- --host 0.0.0.0 --port 5173' > /tmp/weddingdb-dev.log 2>&1 & disown
```

## Reference docs

- [`docs/specs/jwt-revocation-spec.md`](../specs/jwt-revocation-spec.md): JWT refresh rotation and revocation design
- [`docs/adr/002-otel-tracing.md`](../adr/002-otel-tracing.md): OpenTelemetry tracing design
