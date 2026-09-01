# WeddingDB — Developer Documentation

Chinese wedding seating & reservation management system.

- **Frontend:** SvelteKit SPA (Svelte 5 runes + Tailwind CSS v4 + TypeScript)
- **Backend:** Go REST API (Fuego + GORM + PostgreSQL)
- **Realtime:** Redis (DragonflyDB-compatible) pub/sub + SSE

---

## Project Structure

```
├── frontend/       # SvelteKit SPA (SSR disabled, client-only SPA)
├── backend/        # Go REST API (Fuego + GORM + PostgreSQL)
├── docs/dev/       # Developer documentation
└── ui-prototype/   # HTML/CSS/JS design preview (gitignored)
```

## Tech Stack

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
- Docker Compose (see `docker-compose.yml`; Redis service runs DragonflyDB), distroless image, timezone configured.
- Images are published to GitHub Container Registry (`ghcr.io/darknesskiller/weddingdb`) by the Release workflow on `v*` tags (multi-arch: amd64 + arm64). The compose file pulls `ghcr.io/darknesskiller/weddingdb:latest` instead of building locally.
- CI (`ci.yml`) runs on push/PR to `main`: backend `go test ./...`, frontend `npm run check` + `vitest run` + `npm run build`.

## Deployment (Docker Compose)

The compose stack runs three services: `app` (the WeddingDB image), `postgres`, and `redis` (DragonflyDB). The app image is pulled from `ghcr.io/darknesskiller/weddingdb` — CI builds and publishes it on `v*` tags, so there's no local build step.

### Quick start

```sh
# Set a real secret (required)
export JWT_SECRET=$(openssl rand -hex 32)

docker compose up -d
```

- App: `http://localhost:8080` (serves the SPA + API)
- Postgres: `localhost:5432` (user/pass/db all `weddingdb`)
- Redis: `localhost:6379`

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
| `JWT_SECRET` | `change-me-in-production` | **Must set in production** |
| `ADMIN_BOOTSTRAP_PASSWORD` | — | First-boot admin password (required when DB is empty) |
| `TZ` | `Asia/Kuala_Lumpur` | Report timestamps follow this |

### Building locally instead

If you don't want the released image, build from source:

```sh
docker compose build app
# then either run that build:
docker compose up -d
# or swap the compose file to `build: .` and remove `image:`
```

### Data & persistence

- `weddingdb_pgdata` volume — PostgreSQL data (survives `down`, removed with `down -v`)
- `weddingdb_uploads` volume — uploaded kiosk logos/backgrounds, mounted at `/app/uploads`
- Backups: `./backup-db.sh` dumps the `postgres` container to `./backups/weddingdb_<timestamp>.sql.gz`. Note it expects the container named `weddingdb-postgres-1` (default compose naming).

### Updating

```sh
docker compose pull app && docker compose up -d
```

New releases are tagged `v*`; `latest` tracks the newest tag. Pin to a specific version with `ghcr.io/darknesskiller/weddingdb:v0.1.0` for reproducibility.

---

## Getting Started (dev)

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

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | — |
| `REDIS_URL` | Redis connection string | `redis://localhost:6379` |
| `JWT_SECRET` | Secret for JWT signing (required) | — |
| `PORT` | Server port | `8080` |
| `PUBLIC_URL` | Public server URL (used in OpenAPI spec) | `http://localhost:{PORT}` |
| `CORS_ORIGIN` | Allowed CORS origin | `http://localhost:5173` |
| `OTEL_ENABLED` | Enable OpenTelemetry tracing | `false` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint | `http://localhost:4318/v1/traces` |
| `OTEL_LOG_SQL` | `all` records SQL text on `gorm.query` spans (may contain PII) | off |
| `OTEL_LOG_BODY` | `all` records JSON request/response bodies on request spans (passwords masked) | off |

### Tracing (Jaeger)

Opt-in tracing with a local Jaeger. The compose file ships a `jaeger` service behind the `tracing` profile:

```sh
docker compose --profile tracing up -d jaeger
# then set on the app service:
#   OTEL_ENABLED: "true"
#   OTEL_EXPORTER_OTLP_ENDPOINT: "http://jaeger:4318/v1/traces"
docker compose up -d app
```

Jaeger UI at `http://localhost:16686`. Request spans record route pattern, status code, and optionally bodies as span events; every DB query gets a `gorm.query` span with rows returned and, with `OTEL_LOG_SQL=all`, the SQL text as an event. Passwords in captured bodies are masked as `***`.

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

## API Endpoints

### Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Login, returns access + refresh tokens |
| POST | `/api/auth/register` | Register new user account |
| POST | `/api/auth/refresh` | Refresh access token |
| POST | `/api/auth/logout` | Revoke refresh + blacklist access token |
| POST | `/api/auth/select-wedding` | Select wedding context |
| POST | `/api/auth/change-password` | Change password |

### Admin & Users

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
| POST | `/api/upload` | Upload file (max 5MB, images only) |

### Tables, Guests & Seating

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
| POST | `/api/weddings/{wid}/guests/{id}/checkin` | Check in guest |
| POST | `/api/weddings/{wid}/guests/{id}/checkout` | Check out guest |
| POST | `/api/weddings/{wid}/guests/{id}/seat` | Assign seat |
| POST | `/api/weddings/{wid}/guests/import` | Bulk import guests (max 1000) |
| GET | `/api/weddings/{wid}/guests/search?q=` | Search guests |
| GET | `/api/weddings/{wid}/occupancy` | Table occupancy |
| GET | `/api/weddings/{wid}/layout` | Get hall layout (tables + elements with positions) |
| PATCH | `/api/weddings/{wid}/layout` | Save hall layout (atomic replace) |

### Reports & Realtime

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/weddings/{wid}/reports/angpao?format=csv\|xlsx` | Export angpao report (CSV or Excel) |
| GET | `/api/weddings/{wid}/events` | SSE stream for real-time guest events (auth via `?token=`) |

### Public Endpoints (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/public/weddings/{wid}/guests` | List guests (public view) |
| GET | `/api/public/weddings/{wid}/guests/search?q=` | Search guests (public) |
| GET | `/api/public/weddings/{wid}/tables` | List tables (public) |
| GET | `/api/public/weddings/{wid}/kiosk` | Get kiosk settings |
| GET | `/api/public/weddings/{wid}/layout` | Get hall layout (public view) |

---

## Background Dev Server

```sh
nohup sh -c 'cd frontend && npm run dev -- --host 0.0.0.0 --port 5173' > /tmp/weddingdb-dev.log 2>&1 & disown
```

## Reference Docs

- [`docs/specs/jwt-revocation-spec.md`](../specs/jwt-revocation-spec.md) — JWT refresh rotation & revocation design
- [`docs/adr/002-otel-tracing.md`](../adr/002-otel-tracing.md) — OpenTelemetry tracing design
