import { apiFetch } from '#lib/api/client.ts';
import { addToast } from '#lib/stores/index.ts';
import { QUEUE_PREFIX, STAFF_CACHE_PREFIX, KIOSK_CACHE_PREFIX } from '#lib/offline/storage.ts';

export type SyncOp = 'create' | 'update' | 'delete' | 'checkin' | 'checkout';

export interface QueuedMutation {
	mutationId: string;
	op: SyncOp;
	guestId: string;
	clientUpdatedAt: string;
	baseUpdatedAt?: string | null;
	payload?: Record<string, unknown> | null;
	/** Failed server attempts; dropped once it reaches the limit. */
	attempts?: number;
}

const QUEUE_LIMIT = 500;
const MAX_ATTEMPTS = 5;

function qkey(wid: string) { return `${QUEUE_PREFIX}${wid}`; }
function ckey(wid: string) { return `${STAFF_CACHE_PREFIX}${wid}`; }
function kkey(wid: string) { return `${KIOSK_CACHE_PREFIX}${wid}`; }

export function getQueue(wid: string): QueuedMutation[] {
	if (typeof window === 'undefined' || !wid) return [];
	try { return JSON.parse(localStorage.getItem(qkey(wid)) || '[]'); } catch { return []; }
}
function setQueue(wid: string, q: QueuedMutation[]) {
	localStorage.setItem(qkey(wid), JSON.stringify(q.slice(0, QUEUE_LIMIT)));
}

export function enqueue(wid: string, m: QueuedMutation) {
	const q = getQueue(wid);
	if (q.length >= QUEUE_LIMIT) {
		// Refuse instead of shifting: the oldest entry is an unsynced edit.
		throw new Error(`Offline queue is full (${QUEUE_LIMIT}); sync before adding more edits.`);
	}
	q.push(m);
	setQueue(wid, q);
	notifyChanged();
}

export function dequeueByIds(wid: string, ids: string[]) {
	const s = new Set(ids);
	setQueue(wid, getQueue(wid).filter(x => !s.has(x.mutationId)));
	notifyChanged();
}

export function clearQueue(wid: string) {
	localStorage.removeItem(qkey(wid));
	notifyChanged();
}

export function queuedCount(wid: string): number {
	return getQueue(wid).length;
}

function writeCache(store: Storage, key: string, guests: unknown[]) {
	try { store.setItem(key, JSON.stringify(guests)); } catch {}
}
function readCache<T>(store: Storage, key: string): T[] | null {
	try {
		const v = store.getItem(key);
		return v ? JSON.parse(v) as T[] : null;
	} catch { return null; }
}

/** Staff guest records for offline reads; tab-scoped, cleared on logout. */
export function cacheGuests(wid: string, guests: unknown[]) { writeCache(sessionStorage, ckey(wid), guests); }
export function loadCachedGuests<T>(wid: string): T[] | null { return readCache<T>(sessionStorage, ckey(wid)); }

/** Public kiosk records; kept apart from the staff cache. */
export function cacheKioskGuests(wid: string, guests: unknown[]) { writeCache(localStorage, kkey(wid), guests); }
export function loadCachedKioskGuests<T>(wid: string): T[] | null { return readCache<T>(localStorage, kkey(wid)); }

export interface SyncOutcome {
	applied: number;
	skipped: number;
	results: unknown[];
}

// One sync per wedding at a time. The layout fires a sync from several places
// (mount, online, visibility), and overlapping calls replayed the same queue,
// duplicating append-only effects such as check-in notes.
const inFlight = new Map<string, Promise<SyncOutcome>>();

export function syncQueue(wid: string): Promise<SyncOutcome> {
	const running = inFlight.get(wid);
	if (running) return running;
	const run = runSync(wid).finally(() => {
		if (inFlight.get(wid) === run) inFlight.delete(wid);
	});
	inFlight.set(wid, run);
	return run;
}

async function runSync(wid: string): Promise<SyncOutcome> {
	const q = getQueue(wid);
	if (!q.length) return { applied: 0, skipped: 0, results: [] };
	const res = await apiFetch(`/api/weddings/${wid}/guests/sync`, {
		method: 'POST',
		body: JSON.stringify({ mutations: q })
	});
	if (!res.ok) {
		const t = await res.text().catch(() => '');
		throw new Error(`sync failed ${res.status} ${t}`);
	}
	const data = await res.json() as { results: { guestId: string; status: string; serverRecord?: unknown; reason?: string }[] };
	const appliedIds: string[] = [];
	const failedIds: string[] = [];
	let applied = 0, skipped = 0;
	for (let i = 0; i < q.length && i < data.results.length; i++) {
		const r = data.results[i];
		if (r.status === 'applied') { applied++; appliedIds.push(q[i].mutationId); }
		else if (r.status === 'skipped') { skipped++; appliedIds.push(q[i].mutationId); }
		else if (r.status === 'failed') failedIds.push(q[i].mutationId);
	}
	// drop processed (both applied and skipped); the server already holds the newer record
	dequeueByIds(wid, appliedIds);
	const dropped = bumpAttempts(wid, failedIds);
	// Staff must know when offline edits did not land, so they can re-enter them.
	if (skipped) addToast(`${skipped} offline edit(s) were older than the server copy and were not applied`, 'error');
	if (dropped) addToast(`${dropped} offline edit(s) failed ${MAX_ATTEMPTS} times and were discarded`, 'error');
	return { applied, skipped, results: data.results };
}

// bumpAttempts counts a failed server attempt per mutation and drops entries
// that keep failing with the same validation error, so they cannot retry
// forever and block nothing else.
function bumpAttempts(wid: string, ids: string[]): number {
	if (!ids.length) return 0;
	const set = new Set(ids);
	let dropped = 0;
	const kept = getQueue(wid).flatMap(m => {
		if (!set.has(m.mutationId)) return [m];
		const attempts = (m.attempts ?? 0) + 1;
		if (attempts >= MAX_ATTEMPTS) {
			dropped++;
			return [];
		}
		return [{ ...m, attempts }];
	});
	setQueue(wid, kept);
	notifyChanged();
	return dropped;
}

// Same-tab notification that the queue changed (storage events only fire in
// other tabs). The [wid] layout listens for this to update its banner.
function notifyChanged() {
	if (typeof window === 'undefined') return;
	window.dispatchEvent(new Event('offline-queue-changed'));
}
