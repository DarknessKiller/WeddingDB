// localStorage keys for offline data. Kept free of imports so the auth store
// can clear staff data on logout without an import cycle through the API client.

export const QUEUE_PREFIX = 'offline_queue_';
// Staff guest records: email, notes, angbao, check-in times. Cleared on logout.
export const STAFF_CACHE_PREFIX = 'offline_cache_';
// Public kiosk records (the stripped projection). Separate key so the public
// kiosk never reads staff data and never overwrites the staff cache.
export const KIOSK_CACHE_PREFIX = 'kiosk_cache_';

/** Remove every wedding's staff guest cache and unsynced offline edits. */
export function clearStaffOfflineData() {
	if (typeof window === 'undefined') return;
	try {
		const doomed: string[] = [];
		for (let i = 0; i < localStorage.length; i++) {
			const k = localStorage.key(i);
			if (k && (k.startsWith(QUEUE_PREFIX) || k.startsWith(STAFF_CACHE_PREFIX))) doomed.push(k);
		}
		for (const k of doomed) localStorage.removeItem(k);
		// Lets the pending-edits badge drop to zero.
		window.dispatchEvent(new Event('offline-queue-changed'));
	} catch {}
}
