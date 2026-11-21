# Retro: multi-tab and multi-device per account

Date: 2026-11-22. Branch: `DarknessKiller/audit-repo-issues`.

Rules live in [`CODING_STANDARDS.md#multi-tab-and-multi-device`](../../CODING_STANDARDS.md#multi-tab-and-multi-device); this file only records why they exist and where they stop.

## Requirement

One account works from any number of devices, and one browser works with any number of tabs. Staff run the guest list on a phone at the door, a laptop at the desk and a tablet at check-in; the same laptop also has the kiosk preview open. The rule was written down after the code had already grown the behavior, because "the session" and "the queue" were ambiguous.

## Why the guard exists

A tab's refresh response can land after another tab logged in as a different user. Before `160d8e3`, `setAuth` wrote the stale user's session over the new one; the new tab then looked like a foreign session and was logged out. `writeStoredAuth` now refuses to overwrite a stored session that belongs to another user when this tab already had a session. The intentional login path (no session in memory after logout or expiry) still replaces the stored auth.

Earlier fixes that the standard now assumes: end this tab's session when another tab logs out (`06b944d`), never adopt another user's tokens (`2dfbb9a`), keep a newer session when the stored refresh token is this tab's own (`943251f`), 20 SSE streams per user (`bd72117`), re-select the wedding when the stream gets 403 (`80d899a`), plus the offline queue lock and the roster-cache retention commits.

## Known limits

- One browser has one active wedding: its tabs share the refresh token, so select-wedding rescopes all of them. Tabs left on another wedding re-pick their URL wedding on the next 403.
- Browsers without `navigator.locks` fall back to per-tab `inFlight`, so two tabs can sync concurrently. Server replay rules must stay idempotent for that.
- The offline queue belongs to the browser profile, not the account or the device: another user logging in on the same browser is asked before it is discarded; another device has its own queue.
- At most 20 SSE streams per user; the oldest is evicted. Several tabs across several devices count toward that.
