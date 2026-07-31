# WeddingDB

Chinese wedding seating & reservation management system.

## Project Structure

```
├── frontend/       # SvelteKit SPA (Svelte 5 + Tailwind CSS v4)
├── backend/        # Go REST API (Fuego + GORM + PostgreSQL)
└── ui-prototype/   # HTML/CSS/JS design preview (gitignored)
```

## Tech Stack

### Frontend
- **Framework:** SvelteKit (SSR disabled, client-only SPA)
- **UI:** Svelte 5 (runes), Tailwind CSS v4, TypeScript
- **Validation:** Zod v4
- **Icons:** Lucide
- **Fonts:** Inter + Noto Serif SC (Google Fonts)
- **Theme:** Deep Red (#A11217), Gold (#D4AF37), White, Light Beige

### Backend
- **Language:** Go 1.22+
- **HTTP:** Fuego
- **ORM:** GORM + PostgreSQL
- **Cache:** Redis (nonce/replay prevention, access token blacklist)
- **Auth:** JWT HS256 + bcrypt + refresh token rotation

## Getting Started

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
| `CORS_ORIGIN` | Allowed CORS origin | `http://localhost:5173` |

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

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Login, returns access + refresh tokens |
| POST | `/api/auth/register` | Register new user account |
| POST | `/api/auth/refresh` | Refresh access token |
| POST | `/api/auth/logout` | Revoke refresh and access tokens |
| POST | `/api/auth/select-wedding` | Select wedding context |
| POST | `/api/auth/change-password` | Change password |
| GET | `/api/users` | List admin users |
| POST | `/api/users` | Create admin user |
| DELETE | `/api/users/{id}` | Delete admin user |
| GET | `/api/users/{id}/weddings` | Get user wedding assignments |
| PUT | `/api/users/{id}/weddings` | Assign weddings to user |
| PUT | `/api/users/{id}/role` | Update user role |
| POST | `/api/users/{id}/reset-password` | Reset user password |
| GET | `/api/weddings` | List weddings |
| POST | `/api/weddings` | Create wedding |
| GET | `/api/weddings/{id}` | Get wedding |
| PUT | `/api/weddings/{id}` | Update wedding |
| PUT | `/api/weddings/{id}/kiosk` | Update kiosk settings |
| DELETE | `/api/weddings/{id}` | Delete wedding |
| POST | `/api/upload` | Upload file (max 5MB, images only) |
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
| GET | `/api/weddings/{wid}/reports/angpao?format=csv\|xlsx` | Export angpao report (CSV or Excel) |

### Public Endpoints (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/public/weddings/{wid}/guests` | List guests (public view) |
| GET | `/api/public/weddings/{wid}/guests/search?q=` | Search guests (public) |
| GET | `/api/public/weddings/{wid}/tables` | List tables (public) |
| GET | `/api/public/weddings/{wid}/kiosk` | Get kiosk settings |
| GET | `/api/public/weddings/{wid}/layout` | Get hall layout (public view) |

## Background Dev Server

```sh
nohup sh -c 'cd frontend && npm run dev -- --host 0.0.0.0 --port 5173' > /tmp/weddingdb-dev.log 2>&1 & disown
```
