# WeddingDB frontend

SvelteKit SPA for Chinese wedding seating and reservation management. Svelte 5 runes, TypeScript, Tailwind CSS v4. Client only: SSR is disabled.

Product overview: [root README](../README.md). Setup, env vars, and deployment: [developer documentation](../docs/dev/README.md).

## Quick start

```sh
npm install
npm run dev
```

Serves at `http://localhost:5173`. Vite proxies `/api` and `/uploads` to the Go backend at `http://localhost:8080`, so start the backend first.

## Scripts

| Command | What it does |
|---|---|
| `npm run dev` | Vite dev server |
| `npm run build` | Production build |
| `npm run preview` | Serve the production build |
| `npm run check` | `svelte-check` type check |
| `npm run check:watch` | Type check in watch mode |
| `npx vitest run` | Unit tests |

## Layout

```
src/
├── routes/            SvelteKit pages; [wid]/ holds the per-wedding app
├── lib/api/           Typed API client (fetch, SSE, error mapping)
├── lib/offline/       LocalStorage queue for check-ins made without a network
├── lib/stores/        Svelte stores (auth, wedding, guest events)
├── lib/components/    Shared components (seating, layout, ui)
├── lib/utils/         Search, seat formatting, motion, focus helpers
└── app.css            Tailwind entry and theme tokens
```

## Theme

Tailwind v4 tokens live in `src/app.css`. Deep red `#A11217`, gold `#D4AF37`, white, light beige. Fonts are Inter and Noto Serif SC.
