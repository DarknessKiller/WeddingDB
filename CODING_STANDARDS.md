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
- Error strings start lowercase and carry no trailing punctuation: `fmt.Errorf("assign seat: %w", err)`, never `fmt.Errorf("Assign seat: %w", err)`. Acronyms and proper nouns keep their case (`invalid UUID`, `upstream returned 502`). staticcheck `ST1005` enforces this through `golangci-lint` in the verify block.

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

## Offline sync

Files: `backend/internal/services/guest_service.go` (`Sync*`, `syncCheckIn`), `backend/internal/repository/guest_repo.go` (`Sync*`), `frontend/src/lib/offline/**`, `frontend/src/lib/stores/guestEvents.ts`.

- Rules: [`docs/specs/offline-sync-spec.md#invariants`](docs/specs/offline-sync-spec.md#invariants). A change keeps every invariant, or updates the invariant, its test and the spec in the same diff.
- Review and audit sync changes against that list. A behavior the list does not cover is a question for the owner, not a bug to fix.
- A test asserting a rule carries the rule's name. A test that disagrees with an invariant is a bug in the test.

### Verify

```sh
cd backend && CGO_ENABLED=1 go test -run 'Sync' ./internal/services/
```

## Frontend input limits

Files: `frontend/src/lib/utils/limits.ts` (`LIMITS`), every `<input>`/`<textarea>` whose value the API validates, `backend/internal/services/limits.go` (`Max*Len`, `Validate*Text`).

- Every backend character cap has a matching `maxlength` on the input that feeds it, and both read the same number from `#lib/utils/limits.ts`. Never type the number into a template.
- A field the API rejects is not a field the form may accept: online the whole submit fails, offline the edit retries until the queue drops it.
- HTML `maxlength` counts UTF-16 units and Go counts runes, so HTML is stricter for emoji, never looser. Passwords are ASCII only, 8 to 72 characters: `validatePassword` rejects every byte outside `!` to `~` (letters, digits, ASCII punctuation; no space, no Unicode), so bytes and characters stay the same number and bcrypt's 72-byte ceiling equals `MaxPasswordLen`. The form refuses the same set (`PASSWORD_PATTERN`); a non-ASCII password never reaches the API.

### Verify

```sh
rg -n "maxLen\(|Max[A-Za-z]+Len" backend/internal/services backend/internal/handlers
rg -n "maxlength" frontend/src
```

Every cap on the first line has an input on the second. Adding a cap or a form input means changing both.

## Multi-tab and multi-device

Files: `frontend/src/lib/stores/index.ts`, `frontend/src/lib/api/client.ts`, `frontend/src/lib/offline/**`, `backend/internal/services/auth_service.go`, `backend/internal/repository/token_repo.go`.

One account may be signed in on any number of devices, and one browser may open any number of tabs. Say which scope a piece of state belongs to.

- **Per device (session):** one refresh token per login. Never store a single "current token" per user; logout ends that token, and only a security action (password, role, wedding grant) revokes all of them.
- **Per browser (localStorage):** the stored auth, the unsynced queue and the kiosk cache. Tabs share it: read it again after an `await` before writing, listen for the `storage` event, and never adopt another user's stored auth. A late response must not overwrite a session another tab has since replaced.
- **Per tab (sessionStorage or memory):** cached roster data and UI state. Tabs must survive another tab rotating the token and must not render a previous user's data after a login switch.
- **Per connection:** live updates. Every tab and device holds its own SSE stream; an echo of this client's own write is an upsert, and a reconnect reseeds instead of assuming it missed nothing.
- One browser has one active wedding because its tabs share the refresh token. Do not build a feature that needs two weddings open in one browser.
- Only one tab syncs a wedding's queue (`navigator.locks`, `runSync` re-reads inside the lock). Where Web Locks are unavailable, server replay handling stays idempotent.
- Concurrent edits from different devices merge per field by last write; a replayed mutation is `skipped`, never applied twice.

### Verify

```sh
cd backend && go test -run 'TwoDevices|OtherDeviceScope|ReplayWithinGrace' ./internal/services/
cd frontend && npx vitest run src/lib/stores/auth.test.ts src/lib/offline/
```

Add a test with every new rule about session scope, token rotation, or the shared queue.

## Frontend pagination

Files: `frontend/src/lib/api/*.ts`, `frontend/src/routes/[wid]/**/+page.svelte`.

Canonical implementation: `frontend/src/routes/[wid]/guests/+page.svelte`. Copy it; when a list needs something it lacks, change this doc first.

- Lists page by cursor: request `limit` + `cursor`, read `{ <resource>, total, nextCursor }`, and treat an empty `nextCursor` as the last page. The cursor is opaque, so pass through what the server returned.
- API layer: one `listX(weddingId, { limit, cursor })` for a page, plus a `fetchAllX` that drains cursors (`while (cursor)`, `limit: 500`; the server caps a page at 500, so 1,000 guests take two requests). See `listGuests` / `fetchAllGuests` in `frontend/src/lib/api/guests.ts`. Encode the cursor with `encodeURIComponent` when you build a query string by hand.
- UI layer: filter and sort the full list first, then slice it for display. `currentPage` is 0-based `$state`; `pageSize` is 20; `hasNextPage` and `totalPages` are `$derived` (`(currentPage + 1) * pageSize < filtered.length`, `Math.ceil(filtered.length / pageSize)`), never stored copies.
- Reset `currentPage = 0` when search text, filters, sort order, or the row drawer change the visible set.
- Footer sits inside the table card's bottom border: `Page {currentPage + 1} of {totalPages} · {n} guests` on the left, icon-only prev/next buttons on the right with `aria-label="Previous page"` / `aria-label="Next page"` and `disabled` at both bounds. No numbered page list, no page-size selector, no "load more" button.
- Drain the cursor on any unbounded list; never request one large page and drop the rest.

### Verify

```sh
cd frontend && npm run check && npx vitest run
```


