# Retro: frontend input limits

Date: 2026-11-22. Branch: `DarknessKiller/audit-repo-issues`.

## What went wrong

The backend caps text fields (`Max*` constants in `backend/internal/services/guest_service.go`, `maxLen(...)` calls in `backend/internal/handlers/*.go`). Only two frontend inputs enforced the same cap: the notes textarea and the check-in note. Guest name, email, gift, table name, kiosk description, layout element name, wedding name, account name/email and every password field accepted text the API refuses.

Consequences:

- Online: the server answers 400 and the whole form submit fails after the user filled everything.
- Offline: the edit goes into the queue, is rejected on every retry, and burns the retry budget until it is dropped. With per-field sync the other fields of that edit do reach the server, but the rejected field silently never lands.

Same class as the decimal `pax`/angbao bug fixed in `Drawer.svelte`: the form accepted a value the API was always going to refuse.

## Rule

Every backend character cap has a matching `maxlength` on the input that feeds it, and the number lives once in `frontend/src/lib/utils/limits.ts` (mirrors the backend constant). A template never hardcodes the number. Documented in `CODING_STANDARDS.md` under "Frontend input limits", with the verify commands.

`MAX_GUEST_NOTES_LEN` in `frontend/src/lib/api/guests.ts` now re-exports `GUEST_NOTES_MAXLEN` from the same module, so `noteRoom` and the notes textarea cannot drift.

## Inventory after the fix

| Backend cap | Value | Frontend inputs |
|---|---|---|
| `MaxGuestNameLen` | 200 | guest drawer name, reservation name |
| `MaxGuestEmailLen` | 255 | guest drawer email, reservation email |
| `MaxGuestNotesLen` | 2000 | guest drawer notes, reservation notes, check-in note (dynamic room) |
| `MaxGuestGiftItemLen` | 200 | guest drawer gift |
| `MaxGuestDietaryItemLen` | 100 (×20 entries) | dietary chips: fixed options, no free text |
| `maxLen("Name", ...)` table | 100 | table drawer name |
| `maxLen("Name", ...)` wedding | 255 | login new-wedding name, settings wedding name, admin weddings modal |
| `maxLen("Venue name", ...)` | 255 | settings venue name |
| `maxLen("Venue address", ...)` | 500 | settings venue address |
| `maxLen("Kiosk description", ...)` | 2000 | settings description |
| `maxLen("Kiosk ... URL", ...)` | 500 | uploads only, no text input |
| `maxLen("Element name", ...)` | 255 | layout editor element name |
| admin name / email | 255 | users create modal, register page |
| `validatePassword` | 8–72 bytes | users create/reset, change-password |

## How to add a field

1. Add the cap constant to `frontend/src/lib/utils/limits.ts` next to its backend source.
2. Add `maxlength={CONST}` to every input that writes that field.
3. Run the verify block in `CODING_STANDARDS.md#frontend-input-limits`: each backend cap has an input on the frontend side.
4. If the field is reachable offline, the cap matters twice: a rejected queued edit never lands.

## Known gaps

- HTML `maxlength` counts UTF-16 code units, Go counts runes. HTML is stricter for emoji, never looser, so textarea caps are safe.
- Passwords: `validatePassword` caps 72 **bytes**; `maxlength={PASSWORD_MAXLEN}` caps 72 units. A password of multi-byte characters can pass the input and be refused by the server. The field cap is a guard, not a proof.
- No automated drift check. A test that parses the Go constants would couple the frontend test run to the backend tree; the standards verify block is the check for now.
