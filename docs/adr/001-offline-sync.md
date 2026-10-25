# ADR 001: Offline guest sync with newer-time-wins

Date: 2026-08-27

## Context

Door team needs to check in guests when venue WiFi drops. App is SvelteKit SPA + Go API + Postgres, real-time via SSE+Redis. At the time of this decision there was no offline support: a failed fetch lost the mutation. The need was queued mutations that survive reload and merge when back, without adding IndexedDB or new infra.

## Decision

- Scope v1 guest-only. Tables/layout need connection.
- Whole-record LWW on `GuestRecord.UpdatedAt`. One timestamp per guest.
- Client sends `clientUpdatedAt` and `baseUpdatedAt`, server applies if client newer.
- Queue in `localStorage` per wedding `offline_queue_{wid}`, FIFO, cap 500, optimistic UI.
- One batch endpoint `POST /api/weddings/{wid}/guests/sync`, per-item `applied|skipped|failed`.
- Online checkin keeps FIFO guard, sync checkin uses LWW.
- Skipped dropped from the queue; a full guest refresh after the batch restores server truth. Toast names the device that already checked the guest in.
- Sync auto on online/visibility/mount + manual, drain before seedGuests, bulk import blocked offline.

## Alternatives considered

- IndexedDB + background sync: bigger, async, needs lib, not needed for <500 items.
- Per-field merge: saves unrelated edits but complexity, not needed until clobber pain seen.
- Server timestamp only: loses client intent ordering, queue order would be lost.
- CRDT: overkill for one-night event.

## Consequences

- No new deps, one new table-less flow.
- Last write wins may clobber unrelated field if two devices edit same guest different fields at close times. Accepted for v1, revisit if support sees it.
- `clientUpdatedAt` is clamped to `now` server-side so a fast client clock cannot win every later comparison.
- `localStorage` sync is synchronous, 5MB limit safe for 500 * ~500B.

## Status

Implemented: `frontend/src/lib/offline/queue.ts`, `backend/internal/services/guest_service.go`, `backend/internal/handlers/guest.go`, `frontend/src/routes/[wid]/+layout.svelte`. Spec: `docs/specs/offline-sync-spec.md`.
