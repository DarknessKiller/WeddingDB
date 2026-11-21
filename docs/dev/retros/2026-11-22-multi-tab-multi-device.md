# Retro: multi-tab and multi-device per account

Date: 2026-11-22. Branch: `DarknessKiller/audit-repo-issues`.

## Requirement

One account works from any number of devices, and one browser works with any number of tabs. Neither is an edge case: staff open the guest list on a phone at the door, a laptop at the desk and a tablet at check-in, and the same laptop has the kiosk preview open in a second tab.

Code written for "the session" or "the queue" must say which one it means:

- **Per device:** a login. Each device gets its own refresh token; revoking one session must not end the others.
- **Per browser profile:** the stored auth, unsynced edits and the kiosk cache (`localStorage`). Every tab of that browser reads and writes the same copy.
- **Per tab:** the roster cache (`sessionStorage`) and every in-memory store. A tab must survive another tab rotating its token, and must not render another user's data after a login switch.

## What already handles this

- Auth lives under one `localStorage` key (`weddingdb_auth`) with the login email, so tabs share the session and can tell a rotation from another user (`frontend/src/lib/stores/index.ts`). A token this tab retired is never mistaken for another tab's (`retiredRefreshTokens`); the server returns the same successor for a replay inside the grace window (`resumeRotation`).
- Another tab's logout or expiry ends this tab too through the `storage` event; another user's login drops this tab's session and navigates to `/login`.
- Wedding scope is stored on the refresh-token row, so selecting a wedding on one device leaves the other devices' scope alone (`TestSelectWedding_LeavesOtherDeviceScope`).
- The offline queue is per browser (`localStorage`), one tab syncs a wedding at a time through a Web Lock, and `runSync` re-reads the queue inside the lock so a second tab only sends what is left (`frontend/src/lib/offline/queue.ts`).
- The roster cache is `sessionStorage`, so two tabs cannot overwrite each other's view; SSE reseeds each tab and every tab gets the events, so a device's own echo is an upsert, not a duplicate.
- Per-field last-write-wins merges concurrent edits from different devices; a mutation replayed by two tabs comes back `skipped`/already applied.

Prior fixes this rests on, from the branch history: end this tab's session when another tab logs out (`06b944d`), never adopt another user's tokens (`2dfbb9a`), keep a newer session when the stored refresh token is this tab's own (`943251f`), 20 SSE streams per user (`bd72117`), re-select the wedding when the stream gets 403 (`80d899a`), lock the offline sync to one tab, and keep local edits in the offline roster cache.

## New guard

A tab's refresh response can land after another tab logged in as a different user. `setAuth` used to write that stale user's session over the new one, and the new tab then looked like a foreign session and was logged out. `writeStoredAuth` now refuses to overwrite a stored session that belongs to another user when this tab already had a session; the intentional login path (no session in memory after logout/expiry) still replaces the stored auth.

## Known limits

- One browser has one active wedding: the refresh token is shared by its tabs, so a select-wedding rescopes the token for all of them. Tabs that are left on different weddings re-pick their URL wedding on the next 403; do not design a feature that requires two weddings open in one browser.
- Browsers without `navigator.locks` fall back to per-tab `inFlight`, so two tabs can sync concurrently. The server replay rules must stay idempotent for that case.
- The offline queue belongs to the browser profile, not the account or the device: another user logging in on the same browser is asked before it is discarded; another device has its own queue.
- At most 20 SSE streams per user; the oldest stream is evicted. Several tabs across several devices count toward that.

## Checklist for new work

1. Does the state live on the server per user? Key it to the session/connection, not to a package-level "current" value.
2. Does the state live in the browser? Decide localStorage (shared with tabs) or sessionStorage (per tab), and handle the `storage` event when another tab can change it.
3. Does a write happen after an `await` in a tab? Re-read the shared copy before writing; another tab may have changed or replaced it.
4. Can two devices or two tabs send the same request? Make the server path idempotent or last-write-wins per field.
5. Does session revocation exist? It targets one refresh token for logout and all of them only for security actions (password, role, grant change).
