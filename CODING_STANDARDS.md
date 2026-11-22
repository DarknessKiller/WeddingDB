# Coding standards

Rules for this repository, read by people and coding agents. They apply to code
you write or change: new files and edited lines. Untouched code is out of scope.

Before editing, find the code with CodeGraph (`codegraph explore "<symbol or question>"`).
Each section below names its files and its verify commands. Run the verify block
for every area you touched before handoff.

## General

Files: all of `backend/` and `frontend/`.

- Follow the existing code first. Where the code is silent, use the idiom of the
  language and framework.
- Keep diffs small. No unrequested refactors, features, or new dependencies.
- Secrets live in env or a vault, never in code or logs.
- Handle every error (see Go errors below). Nothing vanishes silently.

## Go

Files: `backend/**/*.go`.

- Style: the [Uber Go style guide](https://github.com/uber-go/guide/blob/master/style.md).
  Idioms and stdlib usage: the `use-modern-go` skill, at the version in
  `backend/go.mod` (Go 1.27).
- Lint config: `backend/.golangci.yml`. Do not add `//nolint` without a reason
  after it.

### Go errors

- Sentinel errors live in an `errors.go` file in the package that owns them.
  `backend/internal/services/errors.go` exists. `backend/internal/repository/errors.go`
  is created with the first repository sentinel. Existing repository sentinels
  (`ErrInvalidCursor`, `ErrAlreadyCheckedIn`, `ErrTokenClaimed`) move there when
  their file changes. Each message is written once. No `errors.New` next to the
  code that returns it, and no duplicate sentinel in another package. The handler
  maps a sentinel to a typed `fuego` error.
- Wrap with context when the error crosses a layer:
  `fmt.Errorf("assign seat: %w", err)`. Compare with `errors.Is`.
- Log when the work is best-effort and the request must still succeed. Otherwise
  return the error.
- An error nothing can act on carries a one-line reason beside the directive:
  `//nolint:errcheck // reason`.
- Tests fail on an unexpected error with `testutil.NoErr(t, err)`. Do not discard it.

### Go structure

Files: `backend/internal/{handlers,services,repository,models,utils}`.

- Layering: handler → service → repository.
  - A handler decodes the request, validates input, calls services, and maps
    results or errors to `fuego` errors.
  - A service owns business rules and orchestration across repositories.
  - A repository owns gorm queries and takes `ctx`.
  - Wiring lives in `handlers.RegisterRoutes`. Start from `register.go`.
- Constructors inject dependencies: `NewGuestService(guestRepo, tableRepo, sseHub, db)`.
- Package-level vars hold sentinel errors and constant tables only. No mutable state.
- Thread `c.Context()` from the handler through services and repositories.
  `context.Background()` belongs in bootstrap and detached workers.
- `internal/utils` stays pure: no db, no http, no shared state. Model methods
  stay pure too: JSON marshaling and derivation.

### Go tests

- Stdlib `testing` in `internal/<pkg>/*_test.go`. Table-driven with `t.Run` when
  cases repeat. Request/response tests for handler behaviour.
- No new test dependency.
- A test that asserts an invariant carries the invariant's name. A test that
  disagrees with an invariant is a bug in the test.

### Go bitwise ban

Files: `backend/**/*.go`.

Changed Go code carries no `&`, `|`, `^`, `&^`, `<<`, or `>>`. Write:

| Bitwise pattern | Write instead |
|---|---|
| `if flags & X != 0` | compare the field directly, or `slices.Contains` over the allowed values |
| `a \| b` flags | a named stdlib constant, or a helper that names the combined set |
| `x << n` / `x >> n` | `*` and `/` on decimal quantities, or a helper whose name states the arithmetic |
| `^x` mask | a named constant for the mask |
| sort order picked by bit test | compare only present values (`if req.Sort != ""`) |

Ignore hits in strings and comments. Regex literals are strings, so they are not hits.

### Verify

```sh
cd backend && golangci-lint run ./... && go vet ./... && CGO_ENABLED=1 go test -race -p 1 ./...
git diff -U0 origin/master -- 'backend/*.go' | grep '^+' | grep -nE '(<<|>>|&\^|\^| & | \| )'
```

The second command lists added lines that use bitwise operators. Each hit is a
violation or a false positive in a string or comment. No hits means the rule holds.

## Frontend API and pagination

Files: `frontend/src/lib/api/*.ts`, `frontend/src/routes/[wid]/**/+page.svelte`.

- One `listX(weddingId, { limit, cursor })` returns a page with `nextCursor`.
- One `fetchAllX` drains cursors: `while (cursor)`, with `limit: 100`. The server
  default is 100 and its cap is 500 (`handlers/guest.go`).
- Encode the cursor with `encodeURIComponent` when you build a query string by hand.
- Lists load with infinite scroll. Do not add a numbered page list, a page-size
  selector, prev/next buttons, or a "load more" button.
- Reference: `listGuests` and `fetchAllGuests` in `frontend/src/lib/api/guests.ts`.
  Find other callers with CodeGraph.

### Verify

```sh
cd frontend && npm run check && npx vitest run
```

## Frontend input limits

Files: `frontend/src/lib/utils/limits.ts`, every `<input>` and `<textarea>` whose
value the API validates, `backend/internal/services/guest_service.go` (`Max*`
constants), `backend/internal/handlers/*.go` (`maxLen`).

- Every backend character cap has a matching `maxlength` on the input. Both read
  the number from `frontend/src/lib/utils/limits.ts`. Never type the number into
  a template.
- A field the API rejects is not a field the form may accept. Online, the whole
  submit fails. Offline, the edit retries until the queue drops it.
- HTML `maxlength` counts UTF-16 units and Go counts runes, so HTML is stricter
  for emoji, never looser.
- Passwords are ASCII only, 8 to 72 characters. `validatePassword` rejects every
  byte outside `!` to `~`. That keeps bytes and characters equal, which keeps
  bcrypt's 72-byte limit from truncating. `services.HashPassword` pre-hashes
  anything over 72 bytes with SHA-256 and marks the hash `{sha256}`.

### Verify

Find every cap with CodeGraph (`codegraph explore "maxLen Max Len"`), then find each
matching input with CodeGraph (`codegraph explore "maxlength"`). Every cap has an
input. Adding a cap or a form input means changing both.

## Offline sync

Files: `backend/internal/services/guest_service.go` (`Sync*`, `syncCheckIn`),
`backend/internal/repository/guest_repo.go` (`Sync*`), `frontend/src/lib/offline/**`,
`frontend/src/lib/stores/guestEvents.ts`, `frontend/src/lib/api/guests.ts`.

Design record: [ADR 001](docs/adr/001-offline-sync.md) and
[ADR 003](docs/adr/003-per-column-lww.md).

### Model

- Guest-only offline. Tables and layout edits need a connection.
- Mutation ops: `create`, `update`, `delete`, `checkin`, `checkout`.
- `localStorage` key `offline_queue_{wid}`: a FIFO of `QueuedMutation`, capped
  at 500. A full queue drops the oldest mutation with a toast.
- `localStorage` key `offline_cache_{wid}`: the last complete guest list.
- A mutation carries `clientUpdatedAt` and `baseUpdatedAt`.
- Sync runs on `online`, visibility change, mount, and manual retry. The queue
  drains before the guest list is reseeded.
- The kiosk is read-only. Check-in stays on the staff seating screen.
- Bulk import is blocked offline.
- Create uses a client-generated id that is final. No rewrite is needed.

### Per-column last write wins

Online and offline writes use one rule, so a guest's data is the same whichever
device wrote last.

- Each column group keeps its own write time in `guest_records.field_times`
  (JSON map, group name to RFC 3339 time). A column without an entry falls back
  to `updated_at`.
- A write changes only the columns it carries. It wins a group when its time is
  not older than that group's stored time. Ties count as newer.
- Column groups: `tableId` and `seatNum` share one time. Every other column has
  its own time.
- Online writes stamp server time. Offline writes stamp `clientUpdatedAt`, clamped
  to server time when it is in the future.
- Delete is row-level: it applies only when newer than every column write on the row.
- Seat overlap and the guest cap are checked on the merged result, not on each
  mutation alone.
- Status `skipped` means no column won. It returns `serverRecord`, and the client
  reverts.

### Invariants

Reviews and audits of sync code judge against this list. A change to a rule
updates the rule here, its test, and ADR 003 in the same diff. A behaviour this
list does not cover is a question for the owner, not a bug to fix.

1. **Per-column last write wins.** Each group keeps the value of its newest write.
   A write changes only the columns it carries. Target tests:
   `TestSync_NewestWins`, `TestSync_PartialUpdatePreservesFields`.
2. **Seat moves together.** `tableId` and `seatNum` win or lose as one group.
   Target test: `TestSync_TableAndSeatWinTogether` (not written yet).
3. **Undo is neutral.** Check-in writes `checkedInAt`, `rsvp`, and the payload
   fields it carries. Check-out writes `checkedInAt = null`. Both follow the same
   time rule as any edit. Tests: `TestSync_ArrivalAndUndoFollowTime`,
   `TestSync_UndoBeatsUnrelatedNewerEdit`,
   `TestSyncCheckOut_PostgresLosesToCheckInCommittedWhileWaiting`.
4. **Delete is a row write.** A delete applies when newer than every write on the
   row. An older delete fails and stays queued. Test: `TestSync_DeleteLosesLWW_Failed`.
5. **Replay is a no-op.** A mutation already applied reports `applied` and changes
   nothing, notes included. Tests: `TestSync_ReplayOfAppliedBatchIsApplied`,
   `TestSync_CheckInReplayDoesNotDuplicateNotes`, `TestSync_CheckOutReplayIsApplied`.
6. **Clocks never run ahead.** A future `clientUpdatedAt` is clamped to server time.
   A missing one fails. Test: `TestSync_FutureClockClamped`. Target test for the
   missing-time case: `TestSync_ZeroClientTimeFails` (not written yet).
7. **Offline validates like online.** A field the online API refuses fails the
   mutation without retry. Test: `TestSync_UpdateValidatesLikeOnline`.
8. **One bad mutation fails alone.** A body item that cannot be decoded is
   answered `failed` with `retryable: false`. The rest of the batch applies.
   Target test: `TestSync_MalformedMutationFailsAlone` (not written yet).
9. **Queued edits follow their owner.** A different account logging in on the
   device replays every queue first, then asks before discarding what is left.
   Test: `a different user logging in drops the previous user edits` (storage test).

### Data flow

- Enqueue (offline): UI action → `guests.ts` wrapper → fetch throws or
  `navigator.onLine` is false → `queue.ts enqueue(wid, {op, guestId,
  clientUpdatedAt, baseUpdatedAt, payload})` → optimistic patch and a toast
  "queued offline (N)".
- Sync (reconnect): `queue.syncQueue(wid)` → `POST /api/weddings/{wid}/guests/sync`.
  `applied` drops the mutation and keeps the optimistic state until SSE confirms.
  `skipped` reverts the guest to `serverRecord`, drops it, and toasts. `failed`
  keeps it queued. After the batch, `seedGuests(fetchAllGuests)` reloads.
- Network failure detection covers Chrome `Failed to fetch`, Safari `Load failed`,
  and Firefox `NetworkError`.

### Local search and cache

- Kiosk and staff search run in memory. No request while typing.
- Initial load: `GET /api/public/weddings/{wid}/guests` (paged, includes `pinyin`),
  then `cacheGuests(wid, list)`.
- Ranking in `frontend/src/lib/utils/guestSearch.ts`: exact name, name prefix,
  pinyin prefix, name substring, pinyin substring.
- A failed load falls back to `offline_cache_{wid}`. The UI labels it as a cache result.
- Attendance status is staff-only. Public responses never include it.
- `namePinyin` is hidden on the admin model and sent as `pinyin` in guest JSON.

### Edge cases

- Clock skew: `baseUpdatedAt` detects a stale read. Future client clocks are clamped.
- Refresh fails offline: the queue is kept and the header shows the queued count.
  Access tokens are still required to sync.
- SSE echo of your own sync: upserting the same guest is harmless.
- Cold offline start is unsupported until a service worker caches the app shell.
  The app must load once before the network drops.
- Kiosk idle reset: `kioskIdleResetSeconds` (0 = off). Activity re-arms the timer;
  expiry clears the query and selected guest.

### Verify

```sh
cd backend && CGO_ENABLED=1 go test -run 'Sync' ./internal/services/
cd frontend && npx vitest run
```

## Auth and token revocation

Files: `backend/internal/services/auth_service.go`, `backend/internal/middleware/auth.go`,
`backend/internal/handlers/auth.go`, `backend/internal/handlers/admin.go`,
`backend/internal/repository/token_repo.go`.

### Model

- Access JWTs carry `tv` (token version). Redis holds the current version per user
  at `user:{id}:tv`. A missing key means version 0 (lazy init).
- Logout blacklists the access token's `jti` at `blacklist:jti:{jti}` for the
  token's remaining life.
- Admin revoke deletes the user's refresh tokens in Postgres, then increments `tv`.
- Refresh tokens rotate in one Postgres transaction (`TokenRepo.Rotate`). A
  conditional update on `rotated_at IS NULL` lets exactly one concurrent caller win.
  The loser gets the successor token.
- A consumed refresh token returns the same successor for 60 seconds
  (`refreshGraceWindow`). After that it is rejected as replayed. The family is not revoked.
- Consumed rows are removed by `TokenRepo.DeleteExpired` after `expires_at`.

### Rules

- Middleware is fail-closed. A Redis error on the version or blacklist check rejects
  the request with "service temporarily unavailable".
- Logout is best-effort for the blacklist. A Redis error does not block logout. The
  access token lives until its expiry, at most 15 minutes.
- Revoke returns 500 on a Redis error. The admin retries.
- Self-revoke is rejected with a bad request.
- Blacklisting an expired token is a no-op.

### Verify

```sh
cd backend && CGO_ENABLED=1 go test -race -p 1 ./internal/services/ ./internal/handlers/
```

## Multi-tab and multi-device

Files: `frontend/src/lib/stores/index.ts`, `frontend/src/lib/api/client.ts`,
`frontend/src/lib/offline/**`, `backend/internal/services/auth_service.go`,
`backend/internal/repository/token_repo.go`.

- Each tab reads the shared store. Events from another tab or device arrive over
  SSE and update the same store.
- Revocation and refresh rotation apply to every tab through the 401 path in
  `client.ts`. A tab that retries a consumed refresh token gets the successor
  inside the grace window.

### Verify

```sh
cd frontend && npx vitest run
```

## Review rules

- Before handoff, run the verify block for each area you touched.
- Before commit, push, or PR, run the `change-verification` skill, or dispatch it to a subagent.
- Report failures you did not cause as unrelated, with the evidence.
