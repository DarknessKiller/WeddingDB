# Offline Sync — Implementation Spec

Guest-only offline with newer-time-wins. Queued mutations replay when back online.

## Glossary

| Term | Meaning |
|---|---|
| Guest | `GuestRecord` row, scoped to `wedding_id`. Has `UpdatedAt`. |
| Mutation | One offline intent: `create`, `update`, `delete`, `checkin`, `checkout`. Stored in `localStorage` queue per wedding. |
| Queue | `localStorage` key `offline_queue_{wid}`: FIFO array of `QueuedMutation`. Survives reload. Cap 500. |
| Sync | `POST /api/weddings/{wid}/guests/sync` batch. Server applies LWW per item, returns per-item result. |
| Base version | `baseUpdatedAt` client last saw for that guest. If mismatch, still fall back to `clientUpdatedAt > server.UpdatedAt`. |
| Optimistic | UI updates local `guestList` immediately, marks pending. Reverted if server skips. |

## Domain model

```
GuestRecord { id uuid, weddingId uuid, name, pax, tableId?, seatNum?, checkedInAt?, ... , updatedAt time }
QueuedMutation { mutationId uuid, op enum, guestId uuid, clientUpdatedAt iso, baseUpdatedAt iso|null, payload json }
SyncRequest { mutations: QueuedMutation[] }
SyncResult { guestId string, status: applied|skipped, reason?: string, serverRecord?: GuestRecord }
```

Client generates `guestId` for creates. `clientUpdatedAt` is `new Date().toISOString()` at enqueue time. Queue FIFO.

## Decisions (from grill, Q1-14)

- Q1 scope guest-only v1. Tables/layout blocked offline.
- Q2 whole-record LWW, one `updatedAt` per guest.
- Q3 client `clientUpdatedAt` + `baseUpdatedAt` sent, server picks newer.
- Q4 `localStorage` per wedding, batch sync, per-item result, dropped on applied.
- Q5 auto sync on `online`, `visibilitychange`, mount, plus manual button. Optimistic + cache guests + banner.
- Q6 one batch endpoint `POST /api/weddings/{wid}/guests/sync`.
- Q7 FIFO, no server dedup v1, LWW makes retry idempotent.
- Q8 sync uses `apiFetch` refresh first, queue kept if refresh fails.
- Q9 online checkin keeps FIFO `ConditionalCheckIn`, sync checkin is LWW on `checkedInAt`.
- Q10 skipped reverts to server record, toast.
- Q11 delete tombstone with LWW, `pendingDelete` flag until applied.
- Q12 drain sync before `seedGuests` fetch, SSE re-broadcast normal.
- Q13 bulk import blocked offline.
- Q14 cap 500, header badge, `window.__offlineQueue`, console log.

## Files

### Backend

**`backend/internal/services/guest_service.go`** — add `Sync(ctx, weddingID, mutations) ([]SyncResult, error)`. For each mutation in order: fetch guest, if not found and op != create -> skipped. Compare: if `clientUpdatedAt.After(server.UpdatedAt)` apply, else skipped. For LWW apply does `guestRepo.Update` or `Create` or `Delete`. Publish SSE `sync_*` events for applied. For checkin/checkout map to `CheckedInAt` set/clear.

**`backend/internal/handlers/guest.go`** — add `Sync` handler, types `SyncRequest`, `SyncMutation`, `SyncResult`. Parse `wid`, loop service, return results. Add route `POST /guests/sync`.

**`backend/internal/handlers/register.go`** — register `fuego.Post(scoped, "/guests/sync", guestHandler.Sync)`.

**`backend/internal/repository/guest_repo.go`** — no new method, reuse `FindByID`, `Create`, `Update`, `Delete`.

### Frontend

**`frontend/src/lib/offline/queue.ts`** — new. Exports `enqueue(wid, mutation)`, `peek(wid)`, `dequeue(wid, ids)`, `clear(wid)`, `syncQueue(wid)`. Uses `localStorage`. `syncQueue` calls `POST /api/weddings/{wid}/guests/sync` via `apiFetch`. Handles per-item applied/skipped, updates stores, toasts.

**`frontend/src/lib/api/guests.ts`** — wrap `createGuest`, `updateGuest`, `deleteGuest`, `checkInGuest`, `checkOutGuest`, `assignSeat` to try `apiFetch` first, on network error (`TypeError` or `!navigator.onLine`) enqueue and optimistic update `guestList`/`guestMap`. Export `syncOfflineQueue(wid)`.

**`frontend/src/lib/stores/guestEvents.ts`** — add `cacheGuests(wid, guests)` to `localStorage` on `seedGuests`, `loadCachedGuests(wid)` fallback. Export `offlineQueuedCount` writable.

**`frontend/src/lib/api/client.ts`** — no change, `apiFetch` already handles refresh.

**`frontend/src/routes/[wid]/+layout.svelte`** — init queue sync on mount and `online` event, show banner and badge.

No new deps.

## Data flow: enqueue (offline)

```
UI action -> guests.ts wrapper -> fetch -> throws NetworkError / !online
  -> queue.ts enqueue(wid, {op, guestId, clientUpdatedAt, baseUpdatedAt, payload})
  -> optimistic patch guestList/guestMap + toast "queued offline (N)"
```

## Data flow: sync (reconnect)

```
online / visibility / mount -> queue.syncQueue(wid)
  -> apiFetch POST /api/weddings/{wid}/guests/sync {mutations}
  -> for each result: if applied -> drop from queue, keep optimistic (SSE will confirm)
                   if skipped -> revert guest to serverRecord, drop, toast
  -> seedGuests(fetchAllGuests) after batch
```

## Edge cases

- clock skew: baseUpdatedAt detects stale read even if clock fast.
- refresh fails offline: queue kept, banner "login again to sync".
- queue full 500: drop oldest with toast.
- create with temp id: id is final uuid, no rewrite.
- SSE duplicate of own sync: upsert same guest, harmless.
