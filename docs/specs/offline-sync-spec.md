# Offline Sync — Implementation Spec

Guest-only offline with newer-time-wins, a local guest cache, and device-local
search. Queued mutations replay when the network returns.

## Glossary

| Term | Meaning |
|---|---|
| Guest | `GuestRecord` row, scoped to `wedding_id`. Has `UpdatedAt`. |
| Mutation | One offline intent: `create`, `update`, `delete`, `checkin`, `checkout`. Stored in a `localStorage` queue per wedding. |
| Queue | `localStorage` key `offline_queue_{wid}`: FIFO array of `QueuedMutation`. |
| Cache | `localStorage` key `offline_cache_{wid}`: last complete guest list for the wedding. |
| LWW | Last-write-wins on the guest's `UpdatedAt`, with the client timestamp as the candidate. |

## Domain model

```
GuestRecord { id uuid, weddingId uuid, name, namePinyin, pax, tableId?, seatNum?,
              checkedInAt?, notes, ..., updatedAt time }
QueuedMutation { mutationId uuid, op enum, guestId uuid, clientUpdatedAt iso,
                 baseUpdatedAt iso|null, payload json }
SyncRequest { mutations: QueuedMutation[] }
SyncResult { guestId string, status: applied|skipped|failed, reason?: string,
             serverRecord?: GuestRecord }
```

`namePinyin` stays hidden on the admin model by default but is included as
`pinyin` in guest JSON. The kiosk uses it to search Chinese names by romanized
input without a server round trip.

## Decisions

- Guest-only offline. Tables and layout edits need a connection.
- Whole-record LWW on `GuestRecord.UpdatedAt`. One timestamp per guest.
- The client sends `clientUpdatedAt` and `baseUpdatedAt`; the server applies the
  mutation when the client timestamp is not older. A future client clock is
  clamped to server time.
- Queue is FIFO, capped at 500, with an optimistic UI update and a header badge.
- Check-in notes are appended to the guest's existing notes during sync. A
  queued note never replaces notes written elsewhere.
- Bulk import is blocked offline.
- Sync runs on `online`, visibility, mount, and manual retry. The queue drains
  before the guest list is reseeded.
- The kiosk is read-only. Check-in stays on the staff seating screen.

## Files

### Backend

**`backend/internal/services/guest_service.go`** — `Sync(ctx, weddingID,
mutations)` applies each mutation in order. A missing guest is skipped unless
the op is `create`. Older mutations are skipped and return the server record.
Applied check-in/check-out mutations publish SSE events. Check-in notes are
appended with `appendNote`.

**`backend/internal/handlers/guest.go`** — `POST
/api/weddings/{wid}/guests/sync` decodes, validates and returns per-item
results.

**`backend/internal/handlers/public_guest.go`** — public list/search responses
include `pinyin` for local search. Attendance status stays staff-only. The
kiosk settings response includes `kioskIdleResetSeconds`.

**`backend/internal/models/wedding_event.go`** — `KioskIdleResetSeconds int`,
default 0 (off), capped at 1 hour by the update handler.

**`backend/internal/bootstrap/bootstrap.go`** — drops the legacy
`guest_records.phone` column after `AutoMigrate` when present.

### Frontend

**`frontend/src/lib/offline/queue.ts`** — queue read/write, cap, count, guest
cache helpers, and `syncQueue(wid)`.

**`frontend/src/lib/api/guests.ts`** — create/update/delete/check-in/check-out/
seat wrappers try the API first. A network error, including Safari's
`Load failed` and Firefox's `NetworkError`, enqueues and applies an optimistic
patch. `syncOfflineQueue` wraps `syncQueue`.

**`frontend/src/lib/utils/guestSearch.ts`** — normalization and ranking for the
kiosk and staff search. Name beats pinyin; prefix beats substring. No debounce.

**`frontend/src/routes/[wid]/+layout.svelte`** — caches the staff guest list and
falls back to it when the initial fetch fails.

**`frontend/src/routes/kiosk/[wid]/+page.svelte`** — caches the public guest
list, searches locally, labels a cache fallback, and optionally clears itself
after `kioskIdleResetSeconds` of inactivity.

**`frontend/src/routes/[wid]/search/+page.svelte`** — searches the SSE-backed
guest store locally instead of calling the search endpoint per keystroke.

## Data flow: enqueue (offline)

```
UI action -> guests.ts wrapper -> fetch throws / navigator.onLine is false
  -> queue.ts enqueue(wid, {op, guestId, clientUpdatedAt, baseUpdatedAt, payload})
  -> optimistic patch guestList/guestMap + toast "queued offline (N)"
```

## Data flow: sync (reconnect)

```
online / visibility / mount -> queue.syncQueue(wid)
  -> POST /api/weddings/{wid}/guests/sync {mutations}
  -> applied -> drop from queue, keep optimistic state (SSE confirms)
  -> skipped -> revert guest to serverRecord, drop, toast
  -> failed  -> keep queued for the next retry
  -> seedGuests(fetchAllGuests) after batch
```

## Data flow: local search (kiosk and staff)

```
initial load -> GET /api/public/weddings/{wid}/guests (paged, includes pinyin)
             -> cacheGuests(wid, list)
keystroke    -> rankGuests(allGuests, query) in memory
             -> exact name, name prefix, pinyin prefix, name substring, pinyin substring
```

No request is made while typing. A failed load falls back to `offline_cache_{wid}`.

## Data flow: kiosk idle reset

```
loadKioskConfig -> kioskIdleResetSeconds (0 = off)
activity (pointerdown/keydown) -> re-arm timer
timer fires -> clear query and selected guest, re-arm
```

## Edge cases

- Clock skew: `baseUpdatedAt` detects a stale read; future client clocks are
  clamped.
- Refresh fails offline: the queue is kept and the header shows the queued
  count; access tokens are still required to sync.
- Queue full (500): drop the oldest mutation with a toast.
- Create with a client-generated id: the id is final; no rewrite is needed.
- SSE duplicate of your own sync: upserting the same guest is harmless.
- Cold offline start: unsupported until a service worker caches the app shell.
  The app must be loaded once before the network drops.
- Phone removal: the migration reports a warning instead of blocking boot if
  the database user cannot drop the column.
