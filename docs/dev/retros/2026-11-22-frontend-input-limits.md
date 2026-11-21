# Retro: frontend input limits

Date: 2026-11-22. Branch: `DarknessKiller/audit-repo-issues`.

Rules live in [`CODING_STANDARDS.md#frontend-input-limits`](../../CODING_STANDARDS.md#frontend-input-limits); this file only records why they exist and where they stop.

## What went wrong

The backend capped text fields (`Max*` constants in `backend/internal/services/guest_service.go`, `maxLen(...)` calls in `backend/internal/handlers/*.go`). Only two frontend inputs enforced the same cap: the notes textarea and the check-in note. Every other capped field accepted text the API refuses.

- Online: the server answers 400 and the whole form submit fails after the user filled everything.
- Offline: the edit enters the queue, is rejected on every retry, and burns its retry budget until it is dropped. With per-field sync the other fields of that edit do reach the server, but the rejected field silently never lands.
- Same class as the decimal `pax`/angbao bug in `Drawer.svelte`: the form accepted a value the API was always going to refuse.

Passwords had the mirror bug: `validatePassword` measured with `len()`, so the cap was 72 bytes, not 72 characters, and a 72-character Chinese password was rejected. `services.HashPassword` now SHA-256 pre-hashes anything over bcrypt's 72-byte ceiling, so old hashes still verify without migration.

## Known gaps

- HTML `maxlength` counts UTF-16 code units, Go counts runes. HTML is stricter for emoji, never looser.
- No automated drift check. A test that parsed the Go constants would couple the frontend test run to the backend tree; the standards verify block is the check for now.
