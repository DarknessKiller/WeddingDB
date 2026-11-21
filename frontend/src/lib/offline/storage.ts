// Browser storage keys for offline data. Kept free of imports so the auth store
// can clear staff data on logout without an import cycle through the API client.

// Unsynced edits (localStorage): must survive a reload so they still sync.
export const QUEUE_PREFIX = 'offline_queue_';
// Staff guest records (sessionStorage): email, notes, angbao, check-in times.
// Dies with the tab, so a closed or reloaded tab leaves no roster behind.
export const STAFF_CACHE_PREFIX = 'offline_cache_';
// Public kiosk records (the stripped projection). Separate key so the public
// kiosk never reads staff data and never overwrites the staff cache.
export const KIOSK_CACHE_PREFIX = 'kiosk_cache_';
// Email of the user who owns the queued edits on this device.
const OWNER_KEY = 'offline_owner';

function removePrefixed(store: Storage, prefixes: string[]) {
	const doomed: string[] = [];
	for (let i = 0; i < store.length; i++) {
		const k = store.key(i);
		if (k && prefixes.some(p => k.startsWith(p))) doomed.push(k);
	}
	for (const k of doomed) store.removeItem(k);
}

/** Remove every wedding's staff guest cache and unsynced offline edits. */
export function clearStaffOfflineData() {
	if (typeof window === 'undefined') return;
	try {
		// localStorage cache keys are from builds before the cache moved to sessionStorage.
		removePrefixed(localStorage, [QUEUE_PREFIX, STAFF_CACHE_PREFIX, OWNER_KEY]);
		removePrefixed(sessionStorage, [STAFF_CACHE_PREFIX]);
		// Lets the pending-edits badge drop to zero.
		window.dispatchEvent(new Event('offline-queue-changed'));
	} catch {}
}

/**
 * Session expiry (the server rejected the refresh token), not logout. Drops
 * the cached roster but keeps unsynced edits and their owner key, so the same
 * user's queued changes still sync after re-login. A different user logging in
 * clears them through claimStaffOfflineData.
 */
export function expireStaffOfflineData() {
	if (typeof window === 'undefined') return;
	try {
		removePrefixed(localStorage, [STAFF_CACHE_PREFIX]);
		removePrefixed(sessionStorage, [STAFF_CACHE_PREFIX]);
	} catch {}
}

/** Email that owns this device's queued edits, if any. */
export function staffOfflineOwner(): string | null {
	if (typeof window === 'undefined') return null;
	try {
		return localStorage.getItem(OWNER_KEY);
	} catch {
		return null;
	}
}

/**
 * Called on login. Keeps queued edits when the same user logs back in (after a
 * reload, say) so they still sync; clears them when someone else logs in.
 */
export function claimStaffOfflineData(email: string) {
	if (typeof window === 'undefined') return;
	const owner = email.trim().toLowerCase();
	try {
		if (localStorage.getItem(OWNER_KEY) !== owner) clearStaffOfflineData();
		localStorage.setItem(OWNER_KEY, owner);
	} catch {}
}
