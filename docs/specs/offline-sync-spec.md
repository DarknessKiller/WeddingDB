# Offline Sync — Implementation Spec

Guest-only offline with newer-time-wins. Queued mutations replay when back online.

## Glossary

| Term | Meaning |
|---|---|
| Guest | `GuestRecord` row, scoped to `wedding_id`. Has `UpdatedAt`. |
| Mutation | One offline intent: `create`, `update`, `delete`, `checkin`, `checkout`. Stored in `localStorage` queue per wedding. |
| Queue | `localStorage` key `offline_queue_{wid}`: FIFO array of `QueuedMutation`. Survives reload. Cap 500. |
| Sync | `POST /api/weddings/{wid}/guests/sync` batch, max 500 mutations (`400` above that). Server applies LWW per item, returns per-item `applied|skipped|failed`. |
| Base version | `baseUpdatedAt` client last saw for that guest. If mismatch, still fall back to `clientUpdatedAt > server.UpdatedAt`. |
| Optimistic | UI updates local `guestList` immediately. Not reverted per item: a full guest refresh after the batch re-seeds server truth. |

## Domain model

```
GuestRecord { id uuid, weddingId uuid, name, pax, tableId?, seatNum?, checkedInAt?, ... , updatedAt time }
QueuedMutation { mutationId uuid, op enum, guestId uuid, clientUpdatedAt iso, baseUpdatedAt iso|null, payload json }
SyncRequest { mutations: QueuedMutation[] }
SyncResult { guestId string, op enum, status: applied|skipped|failed, reason?: string, serverRecord?: GuestRecord }
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
- Q9 online checkin keeps FIFO `ConditionalCheckIn`, sync checkin is LWW on `checkedInAt`. Both promote `rsvp` to `confirmed`. The optional check-in note is stored in `GuestRecord.checkinNote`, not appended to `notes`.
- Q10 skipped is dropped from the queue; the full refresh that follows the batch restores server truth. A check-in already recorded by another device raises a toast naming that device.
- Q11 delete has no tombstone. A delete older than the server record returns `failed`, not `skipped`, so the client keeps it queued instead of silently losing the intent.
- Q12 drain sync before `seedGuests` fetch, SSE re-broadcast normal.
- Q13 bulk import blocked offline.
- Q14 cap 500, header badge, `window.__offlineQueue`, console log.

## Files

### Backend

**`backend/internal/services/guest_service.go`** (`GuestService.Sync`, `applySyncMutation`) — per mutation, in order: `clientUpdatedAt` is clamped to `now` if in the future, so a fast clock cannot win every later comparison; the op applies unless strictly older than `server.UpdatedAt`, so equal timestamps win the tie-break. Guest not found: `create` proceeds, every other op returns `skipped`. Invalid `guestId`, table/seat validation failure, or transaction error returns `failed`. Writes go through `createGuestTx`/`updateGuestTx`, so the guest cap still applies. Applied ops publish SSE `create|update|delete`.

**`backend/internal/handlers/guest.go`** (`GuestHandler.Sync`) — `SyncRequest{Mutations []services.SyncMutation}`; rejects >500 mutations with `400 Too many mutations (max 500)`; returns `{results: SyncResult[]}`. Route registered in `register.go` as `POST /guests/sync`.

**`backend/internal/handlers/register.go`** — register `fuego.Post(scoped, "/guests/sync", guestHandler.Sync)`.

**`backend/internal/repository/guest_repo.go`** — no new method, reuse `FindByID`, `Create`, `Update`, `Delete`.

### Frontend

**`frontend/src/lib/offline/queue.ts`** — new. Exports `getQueue(wid)`, `enqueue(wid, mutation)`, `dequeueByIds(wid, ids)`, `clearQueue(wid)`, `queuedCount(wid)`, `cacheGuests(wid, guests)`, `loadCachedGuests(wid)`, `syncQueue(wid)`. Uses `localStorage`. `enqueue` drops the oldest entry at 500 and dispatches `offline-queue-changed` on `window` (same-tab signal; `storage` events fire only in other tabs). `syncQueue` POSTs the whole queue via `apiFetch` and dequeues only `applied` and `skipped`; `failed` stays queued for retry. Exposes `window.__offlineQueue` and `window.__syncLastResults` for support.

**`frontend/src/lib/api/guests.ts`** — `createGuest`, `updateGuest`, `deleteGuest`, `checkInGuest`, `checkOutGuest`, `assignSeat`, `unassignSeat` try `apiFetch` first; on network error or `isOffline()` they `enqueue` and optimistic-patch `guestList`/`guestMap`. `syncOfflineQueue(wid)` is a thin re-export of `syncQueue`. `bulkImportGuests` throws offline.

**`frontend/src/lib/stores/guestEvents.ts`** — holds no queue state. Writes `offline_cache_{wid}` inline in the seed path (~line 100) so the list survives a reload while offline; the `[wid]` layout reads that key as the fallback when `fetchAllGuests` fails. There is no `offlineQueuedCount` store: the banner count is local `$state` in the layout, refreshed from `localStorage` on `storage` and `offline-queue-changed`.

**`frontend/src/lib/api/client.ts`** — no change, `apiFetch` already handles refresh.

**`frontend/src/routes/[wid]/+layout.svelte`** — `refreshQueued()` reads queue length for the badge; `doSync()` runs `syncQueue` then `refreshGuests(loadGuests)`, keeping the syncing window open so mid-flight SSE events are replayed after the seed instead of clobbered. Triggers: `online`, `visibilitychange` (visible only), mount, manual button. `offline` flips the banner. `loadGuests` falls back to `offline_cache_{wid}` when the fetch throws.

No new deps.

## Data flow: enqueue (offline)

```
UI action -> guests.ts wrapper -> fetch -> throws NetworkError / !online
  -> queue.ts enqueue(wid, {op, guestId, clientUpdatedAt, baseUpdatedAt, payload})
  -> optimistic patch guestList/guestMap + toast "queued offline (N)"
```

## Data flow: sync (reconnect)

```
online / visibility / mount / manual -> queue.syncQueue(wid)
  -> apiFetch POST /api/weddings/{wid}/guests/sync {mutations} (max 500)
  -> applied  -> dequeue, keep optimistic (SSE confirms)
  -> skipped  -> dequeue, toast if a check-in was already recorded by another device
  -> failed   -> stays queued, retried on the next sync
  -> refreshGuests(fetchAllGuests) re-seeds server truth after the batch
```
## Edge cases

- clock skew: `clientUpdatedAt` is clamped to `now` server-side, so a fast clock cannot win every LWW comparison.
- stale read: `baseUpdatedAt` is sent, but the decision is `clientUpdatedAt` vs `UpdatedAt`; the base version only records what the client last saw.
- refresh fails offline: queue kept, banner "login again to sync".
- queue full 500: oldest dropped on enqueue, console log.
- server rejects >500 mutations in one batch: `400`, client keeps the queue.
- create `guestId` is the final uuid, generated client-side, no rewrite on sync.
- SSE duplicate of own sync: upsert same guest, harmless.
- `failed` (invalid id, table/seat validation, cap, transaction error) stays queued until it applies or the cap drops it.
