# ADR 001: Local-first door operations

Date: 2026-08-27
Amended: 2026-10-31

## Context

The door team checks guests in while venue WiFi comes and goes. The app is a
SvelteKit SPA backed by a Go API, PostgreSQL, Redis pub/sub and SSE.

The first offline version queued guest mutations and replayed them on
reconnect. Lookups still needed a network round trip, the public kiosk did not
cache its guest list, Safari and Firefox network errors could escape the queue,
and the kiosk could leave a guest's name and table on screen indefinitely. The
legacy `phone` column also survived in older databases after the UI stopped
using it.

## Decision

1. **Guest data is local-first at the door.** The staff layout caches the guest
   list in `localStorage`; the kiosk caches the public guest list under the same
   per-wedding key. A failed fetch falls back to the cache instead of an empty
   screen.
2. **Search runs on the device.** The public guest payload includes the
   server-generated `namePinyin`. The kiosk and staff search rank exact name,
   name prefix, pinyin prefix, name substring, then pinyin substring, with no
   debounce and no request per keystroke.
3. **Offline check-in is a queued intent.** Check-in carries an optional note,
   is applied optimistically, survives reload, and replays through
   `POST /api/weddings/{wid}/guests/sync`. A note is appended to existing notes,
   never used to replace them. Network failures are detected across Chrome,
   Firefox and Safari, not only Chrome's `Failed to fetch`.
4. **Kiosk idle reset is opt-in and off by default.** `kioskIdleResetSeconds` is
   stored on the wedding (0 = off). When enabled, the kiosk clears the query and
   selected guest after that much inactivity, so a name does not sit on a public
   screen between arrivals.
5. **Phone numbers are not part of the product.** They are removed from the
   guest table and bulk import payload. Bootstrap drops the legacy
   `guest_records.phone` column when it exists.
6. **The kiosk stays read-only.** Check-in remains a staff action on the seating
   screen. A public terminal can look up a table, but cannot mutate the guest
   list.

## Consequences

- Public guest JSON exposes `pinyin` only. Attendance status (`checkedInAt`)
  stays staff-only; a public kiosk must not reveal who has already arrived.
- A cold start with no network still needs the SPA shell. There is no service
  worker yet, so the app must have been loaded once before the network drops.
  This is the documented limit of the current offline story.
- Cached lists can be stale until the next successful poll or SSE event. The
  kiosk labels a cache fallback as "Offline copy".
- Local search is only as good as the synced pinyin. Guests created offline are
  searchable by name until the sync response returns their pinyin.
- Whole-record last-write-wins remains the merge rule. Per-field merge is still
  deferred until clobbering is observed in real use.

## Alternatives considered

- **Server-side type-ahead:** correct data, unusable latency on venue WiFi and
  no results when the network is down.
- **IndexedDB + background sync:** more storage and async complexity for fewer
  than 1,000 guests; `localStorage` is enough for one night.
- **Client-side pinyin library:** duplicates the server dictionary and adds a
  large dependency; the server already computes the canonical value.
- **CRDT or per-field merge:** overkill for a one-night event with a small door
  team.

## Status

Accepted. Implements `docs/specs/offline-sync-spec.md`.
