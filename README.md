# WeddingDB

Chinese wedding seating & reservation management system.

## Project Structure

```
├── frontend/       # SvelteKit SPA (Svelte 5 + Tailwind CSS v4)
└── ui-prototype/   # HTML/CSS/JS design preview (gitignored)
```

## Tech Stack

- **Framework:** SvelteKit (SSR disabled, client-only SPA)
- **UI:** Svelte 5 (runes), Tailwind CSS v4, TypeScript
- **Validation:** Zod v4
- **Icons:** Lucide
- **Fonts:** Inter + Noto Serif SC (Google Fonts)
- **Theme:** Deep Red (#A11217), Gold (#D4AF37), White, Light Beige

## Getting Started

```sh
cd frontend
npm install
npm run dev
```

Server runs at `http://localhost:5173`.

## Pages

| Route | Description |
|-------|-------------|
| `/dashboard` | Stats overview, recent activity, table occupancy |
| `/guests` | Guest list with search, filter, status badges |
| `/seating` | Interactive banquet hall map (most important page) |
| `/search` | Guest search with check-in workflow |
| `/reservation` | Guest registration with seat picker |
| `/tables` | Table overview with capacity bars |
| `/kiosk` | Standalone guest self-service kiosk |
| `/settings` | App settings (placeholder) |

## Background Dev Server

```sh
nohup sh -c 'cd frontend && npm run dev -- --host 0.0.0.0 --port 5173' > /tmp/weddingdb-dev.log 2>&1 & disown
```
