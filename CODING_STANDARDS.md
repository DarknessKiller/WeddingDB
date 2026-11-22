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
- One shared function per shape conversion. Backend: `guestToEventData` builds SSE
  guest data, `SSEHub.publishGuest` publishes it, `toPublicGuests` builds public
  rows. Frontend: `frontend/src/lib/utils/guestTransform.ts` (`toGuest`,
  `reviveGuest`, `applyGuestPatch`) is the only API-row-to-`Guest` mapping. Do not
  map fields by hand in a page, store, or API wrapper.
- Measure and truncate user text by runes (Go) or code points, never bytes. A byte
  cut can split a CJK character and the database rejects it.

## Go

Files: `backend/**/*.go`.

- Style: the [Uber Go style guide](https://github.com/uber-go/guide/blob/master/style.md).
  Idioms and stdlib usage: the `use-modern-go` skill, at the version in
  `backend/go.mod` (Go 1.27).
- Lint config: `backend/.golangci.yml`. Do not add `//nolint` without a reason
  after it.

### Go errors

- Sentinel errors live in an `errors.go` file in the package that owns them:
  `backend/internal/services/errors.go` and `backend/internal/repository/errors.go`.
  Each message is written once. No `errors.New` next to the
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
- Lock order: a write that touches guests or tables locks the wedding row first,
  then the table row (`lockWedding`, `lockTable` in `services`). Another order
  can deadlock against guest writes.
- A check-then-write guard (last admin, guest cap, seat overlap) runs the check
  and the write in one transaction that holds a row lock. A count outside the
  write's transaction races.
- A missing row on a lookup by id maps to the package's not-found sentinel and
  HTTP 404. It never returns a zero value with a nil error.

### Go tests

- Stdlib `testing` in `internal/<pkg>/*_test.go`. Table-driven with `t.Run` when
  cases repeat. Request/response tests for handler behaviour.
- No new test dependency.
- A test that asserts an invariant carries the invariant's name. A test that
  disagrees with an invariant is a bug in the test.
- Not required, by owner decision: a concurrency test for the SSE keepalive token
  recheck, and a concurrent (Postgres) test for the last-admin lock. The sequential
  `TestAdminRepo_LastAdminGuard` is enough. Do not report their absence as a gap.

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

## Security boundaries

Files: `backend/internal/config/{config,server}.go`,
`backend/internal/handlers/{register,upload,sse,guest}.go`,
`backend/internal/services/{report_service,sse,guest_service}.go`.

- Startup rejects `JWT_SECRET` shorter than 32 characters, missing secrets, and
  shipped placeholder secrets. Generate secrets with `openssl rand -hex 32`.
- JSON request bodies have a 1 MiB limit, including requests without
  `Content-Length`; reject oversized bodies with HTTP 413 before handler side
  effects. Multipart uploads retain their separate file-size limit.
  HTTP `ReadTimeout` is five minutes; `WriteTimeout` stays zero for SSE.
- Untyped errors return a generic HTTP 500. Never expose driver/query details
  through typed error titles, details, or per-item offline-sync failure reasons
  (including HTTP 200 responses). Preserve known validation/business errors;
  log unexpected failures server-side and return a generic message.
- CSV export prefixes text cells starting with `=`, `+`, `-`, or `@` with an
  apostrophe. Apply this to guest names, table names, gifts, and summary rows;
  CSV quoting alone does not prevent formula injection.
- Upload rejects non-admin callers without wedding scope before parsing or
  writing files. Wedding authorization follows the existing JWT scope;
  admins retain cross-wedding access. SSE admits at most 100 concurrent streams
  per wedding and 5 per user across the deployment using atomic Redis leases;
  overflow returns
  HTTP 429 and Redis admission errors return HTTP 503 before stream headers.
  Disconnect releases its lease, crashed servers' leases expire, and active
  streams renew leases. Failed or expired renewal closes the stream; write
  deadlines bound blocked writes to the locally confirmed lease lifetime. Other
  weddings retain independent capacity. Redis must preserve lease keys and its
  clock must not jump forward while streams are active. Each keepalive tick
  re-runs `ValidateToken`; a rejected token closes the stream, an outage keeps it.
- Uploads have a per-file limit (5 MiB) and no rate limit or disk quota, by
  owner decision.

### Verify

Run the Go verify block above. Also run the focused security regressions with
`TEST_DATABASE_URL` pointing to a dedicated PostgreSQL test database so the CSV
integration test executes rather than skips:

```sh
cd backend && CGO_ENABLED=1 go test -race -p 1 ./internal/config ./internal/handlers ./internal/services -run 'TestLoadEnvJWTSecretLength|TestUntypedErrorsAreGeneric|TestRegisterRoutesJSONBodyLimit|TestNewFuegoServer_TimeoutsForLongTransfers|TestUpload_|TestGenerateCSV_FormulaInjection|TestSSEHub_ConcurrentWeddingLimit|TestSSEHub_PerUserLimit|TestSSEWeddingStreamLimit|TestRegisteredGuestSeatDatabaseErrorIsGeneric|TestSyncDatabaseErrorIsGeneric' -count=1
cd backend && TEST_REDIS_ADDR=127.0.0.1:6379 CGO_ENABLED=1 go test -race ./internal/services -run '^TestSSEHub_(DistributedWeddingLimit|RenewalFailureClosesStream)$' -count=1
```

Reviewers must check rejection before side effects, absence of internal error
text, CSV data and summary cells, and SSE capacity release, not only status codes.

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
- Guest store writes go through `upsertGuest` and `removeGuest` in
  `frontend/src/lib/stores/guestEvents.ts`, so `guestList`, `guestMap`, and table
  occupancy stay in step.
- Dialogs use `role="dialog"`, `aria-modal="true"`, an accessible label, Escape to
  close, `use:trapFocus`, and a labeled close button. A clickable row also has a
  real button inside it for keyboard users.

### Verify

```sh
cd frontend && npm run check && npx vitest run
```

## Frontend input limits

Files: `frontend/src/lib/utils/limits.ts` (`LIMITS`),
`backend/internal/services/limits.go` (`Max*Len`, `Validate*Text`,
`MaxGuestPartySize`), every `<input>` and `<textarea>` whose value the API validates.

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

Find every cap in `backend/internal/services/limits.go` and its mirror in
`frontend/src/lib/utils/limits.ts`, then find each matching input with CodeGraph
(`codegraph explore "maxlength"`). Every cap has an input. Adding a cap or a form
input means changing both files and the field that feeds it.

## Offline sync

Files: `backend/internal/services/guest_service.go` (`Sync`, `mergeGuestUpdate`),
`backend/internal/services/guest_lww.go`, `backend/internal/services/guest_patch.go`,
`backend/internal/repository/guest_repo.go` (`FindByIDForUpdate`, `Update`),
`frontend/src/lib/offline/**`, `frontend/src/lib/stores/guestEvents.ts`,
`frontend/src/lib/api/guests.ts`.

Design record: [ADR 001](docs/adr/001-offline-sync.md) and
[ADR 003](docs/adr/003-per-column-lww.md).

### Model

- Guest-only offline. Tables and layout edits need a connection.
- Mutation ops: `create`, `update`, `delete`, `checkin`, `checkout`.
- `localStorage` key `offline_queue_{wid}`: a FIFO of `QueuedMutation`, capped
  at 500. A full queue drops the oldest mutation with a toast.
- `localStorage` key `offline_cache_{wid}`: the last complete staff guest list,
  stored as app-shaped `Guest` objects. Read it back with `reviveGuest`, never
  `toGuest`. The kiosk uses its own key, `kiosk_cache_{wid}`.
- Sync posts at most 100 mutations per request, so a batch stays under the 1 MiB
  body limit.
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
  to `updated_at`; a legacy row seeds every group from `updated_at` on its first
  merged write.
- Presence rule: a write changes only the groups it carries. An absent field
  keeps its stored value, an explicit `null` clears a nullable field, and a
  sparse client write leaves every other group alone.
- Column groups: `name`, `email`, `pax`, `tableId`+`seatNum`, `rsvp`,
  `checkedInAt`, `notes`, `dietary`, `isVip`, `angbaoAmt`, `giftItem`.
  `tableId` and `seatNum` share one time; every other column has its own.
- Online writes stamp server time. Offline writes stamp `clientUpdatedAt`, clamped
  to server time when it is in the future, and a missing timestamp fails.
- A create stamps every group with its operation time.
- Online and offline writes call one merge function. Create-existing-id and the
  online put both take the guest row lock and merge the carried groups.
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
   A write changes only the columns it carries. Tests:
   `TestSync_NewestWins`, `TestSync_PartialUpdatePreservesFields`.
2. **Seat moves together.** `tableId` and `seatNum` win or lose as one group.
   Target test: `TestSync_TableAndSeatWinTogether`.
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
   A missing one fails. Tests: `TestSync_FutureClockClamped`,
   `TestSync_ZeroClientTimeFails`.
7. **Offline validates like online.** A field the online API refuses fails the
   mutation without retry. Test: `TestSync_UpdateValidatesLikeOnline`.
8. **One bad mutation fails alone.** A body item that cannot be decoded is
   answered `failed` with `retryable: false`. The rest of the batch applies.
   Test: `TestSync_MalformedMutationFailsAlone`.
9. **Queued edits follow their owner.** A different account logging in on the
   device replays every queue first, then asks before discarding what is left.
   `claimQueues` runs after wedding selection. The owner is the user id (JWT
   `sub`) at `offline_owner`; the prompt names the previous account by its masked
   email (`j***@x.io`) from `offline_owner_label`. Never store a plain email.
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
- A failed kiosk load falls back to `kiosk_cache_{wid}`. The UI labels it as a cache result.
- Attendance status and RSVP are staff-only. Public responses never include
  `rsvp`, `checkedInAt`, `notes`, or `email`. Test: `TestPublicGuestJSONOmitsPrivateFields`.
- `namePinyin` is hidden on the admin model and sent as `pinyin` in guest JSON.

### Edge cases

- Clock skew: future client clocks are clamped. `enqueue` stamps `baseUpdatedAt`
  with the guest `updatedAt` the device last saw (null for a create). The server
  accepts it but per-column LWW decides; it does not reject stale reads today.
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
- Expired refresh token rows are removed by the boot-time cleanup in
  `backend/internal/bootstrap/bootstrap.go`. Rows pile up between restarts.
- Wedding selection (`POST /api/auth/select-wedding`, body `weddingId` and
  `refreshToken`) scopes only the caller's refresh token. Other sessions of the
  same user keep their scope.

### Rules

- Middleware is fail-closed. A Redis error on the version or blacklist check rejects
  the request with HTTP 503 "service temporarily unavailable".
- Token rejections (bad, expired, revoked, blacklisted, version mismatch, replayed,
  missing row) wrap `services.ErrTokenRejected` and return 401 with a fixed title.
  Any other Redis or DB error returns 503, never 401, so an outage does not log
  staff out. `middleware.WriteAuthError` does the mapping.
- The frontend has one refresh path: `refreshIfNeeded` in
  `frontend/src/lib/api/client.ts`. Only a 401 from refresh clears auth; a 503
  keeps the session. SSE reconnects call it before retrying.
- Tokens live in memory. Never navigate with `window.location.href` while signed
  in; use `goto`.
- Logout is best-effort for the blacklist. A Redis error does not block logout. The
  access token lives until its expiry, at most 15 minutes.
- Revoke returns 500 on a Redis error. The admin retries.
- Self-revoke is rejected with a bad request.
- Blacklisting an expired token is a no-op.
- Deleting a user removes their `user_weddings` grants in the same transaction as
  the account row, then revokes their sessions. The revoke is best-effort and runs
  after the row is gone.
- An admin password reset sets `ForcePasswordChange`. The next change-password call
  skips the current-password proof once, clears the flag, and revokes sessions.
- Revocation deletes refresh tokens before the token-version bump and again after
  it. A refresh token carries the version it was minted at. A token, or successor,
  older than the current version is deleted and rejected.

### Verify

```sh
cd backend && CGO_ENABLED=1 go test -race -p 1 ./internal/services/ ./internal/handlers/ ./internal/middleware/ ./internal/repository/
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
