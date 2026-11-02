# Coding standards

Opinionated consistency rules for this repository. They apply to code you write or change: new files and edited lines. Existing untouched code is not in scope.

Run the verify block of the section you touched before handoff.

## Go

Files: `backend/**/*.go`.

- Follow the [Uber Go style guide](https://github.com/uber-go/guide/blob/master/style.md). Concise, testable code.
- Write explicit comparisons, named standard-library constants, or existing helpers in place of bitwise operations. Changed Go code carries no `&`, `|`, `^`, `&^`, `<<`, or `>>`. During review, treat each new bitwise op as a standards violation and replace it before handoff.
- For optional sort orders, compare only present values.
- Language and stdlib idioms come from the `use-modern-go` skill, at the version in `backend/go.mod` (Go 1.27).
- Handle every error: return it wrapped with context (`fmt.Errorf("assign seat: %w", err)`), or log it when the work is best-effort and the request must still succeed. An error nothing can act on carries a one-line reason beside `//nolint:errcheck`; tests fail on it (`testutil.NoErr(t, err)`) rather than discarding it.

### Replacements

| Bitwise pattern | Write instead |
|---|---|
| `if flags & X != 0` | compare the present field directly, or `slices.Contains` over the allowed values |
| `a \| b` flags | named stdlib constant, or a helper that names the combined set |
| `x << n` / `x >> n` | `/` and `*` on decimal quantities, or a helper whose name states the arithmetic |
| `^x` mask | named constant for the mask |
| sort order picked by bit test | compare only present values (`if req.Sort != ""`) |

### Review a branch

```sh
git diff -U0 origin/master -- 'backend/*.go' | grep '^+' | grep -nE '(<<|>>|&\^|\^| & | \| )'
```

Each hit is either a violation to rewrite or a string/comment false positive to ignore. No hits means the rule holds.

### Verify

```sh
cd backend && golangci-lint run ./... && go vet ./... && CGO_ENABLED=1 go test ./...
```

## Go structure

Files: `backend/internal/{handlers,services,repository,models,utils}`.

- Layering: handler → service → repository. A handler decodes the request, validates input, calls the service methods it needs, and maps results or errors to typed `fuego` errors. A service owns business rules and orchestration across repositories. A repository owns gorm queries and takes `ctx`. Wiring lives in `handlers.RegisterRoutes`: read `register.go`, `guest_service.go` (`MaxGuestsPerWedding`), `guest_repo.go`.
- Constructors inject dependencies (`NewGuestService(guestRepo, tableRepo, sseHub, db)`). Package-level vars hold sentinel errors and constant tables only, never mutable state.
- Thread `c.Context()` from the handler through service and repository calls. `context.Background()` belongs in bootstrap and detached workers.
- Return sentinel errors from the owning layer (`ErrGuestCapExceeded`, `ErrInvalidCursor`, `ErrAlreadyCheckedIn`), add context with `fmt.Errorf("...: %w", err)`, and translate them to typed `fuego` errors at the handler. Errors reach the caller or get logged; none vanish.
- Helpers in `internal/utils` stay pure: no db, no http, no shared state (see `encoding.go`). Model methods stay pure too: JSON marshaling and derivation.
- Tests: stdlib `testing` in `internal/<pkg>/*_test.go`, table-driven with `t.Run` when cases repeat, request/response tests for handler behaviour. No new test dependency.

## Frontend pagination

Files: `frontend/src/lib/api/*.ts`, `frontend/src/routes/[wid]/**/+page.svelte`.

Canonical implementation: `frontend/src/routes/[wid]/guests/+page.svelte`. Copy it; when a list needs something it lacks, change this doc first.

- Lists page by cursor: request `limit` + `cursor`, read `{ <resource>, total, nextCursor }`, and treat an empty `nextCursor` as the last page. The cursor is opaque, so pass through what the server returned.
- API layer: one `listX(weddingId, { limit, cursor })` for a page, plus a `fetchAllX` that drains cursors (`while (cursor)`, `limit: 100`). See `listGuests` / `fetchAllGuests` in `frontend/src/lib/api/guests.ts`. Encode the cursor with `encodeURIComponent` when you build a query string by hand.
- UI layer: filter and sort the full list first, then slice it for display. `currentPage` is 0-based `$state`; `pageSize` is 20; `hasNextPage` and `totalPages` are `$derived` (`(currentPage + 1) * pageSize < filtered.length`, `Math.ceil(filtered.length / pageSize)`), never stored copies.
- Reset `currentPage = 0` when search text, filters, sort order, or the row drawer change the visible set.
- Footer sits inside the table card's bottom border: `Page {currentPage + 1} of {totalPages} · {n} guests` on the left, icon-only prev/next buttons on the right with `aria-label="Previous page"` / `aria-label="Next page"` and `disabled` at both bounds. No numbered page list, no page-size selector, no "load more" button.
- Drain the cursor on any unbounded list. `frontend/src/lib/api/dashboard.ts` still requests `?limit=500` in one shot and drops the rest past 500 guests; fix it when you touch it, and do not copy it.

### Verify

```sh
cd frontend && npm run check && npx vitest run
```


