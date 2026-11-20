import { apiFetch } from '#lib/api/client.ts';
import { QUEUE_PREFIX, STAFF_CACHE_PREFIX, KIOSK_CACHE_PREFIX } from '#lib/offline/storage.ts';

export type SyncOp = 'create' | 'update' | 'delete' | 'checkin' | 'checkout';

export interface QueuedMutation {
	mutationId: string;
	op: SyncOp;
	guestId: string;
	clientUpdatedAt: string;
	baseUpdatedAt?: string | null;
	payload?: Record<string, unknown> | null;
}

function qkey(wid: string) { return `${QUEUE_PREFIX}${wid}`; }
function ckey(wid: string) { return `${STAFF_CACHE_PREFIX}${wid}`; }
function kkey(wid: string) { return `${KIOSK_CACHE_PREFIX}${wid}`; }

export function getQueue(wid: string): QueuedMutation[] {
	if (typeof window === 'undefined' || !wid) return [];
	try { return JSON.parse(localStorage.getItem(qkey(wid)) || '[]'); } catch { return []; }
}
function setQueue(wid: string, q: QueuedMutation[]) {
	localStorage.setItem(qkey(wid), JSON.stringify(q.slice(0, 500)));
}

export function enqueue(wid: string, m: QueuedMutation) {
	const q = getQueue(wid);
	if (q.length >= 500) q.shift();
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
	let applied = 0, skipped = 0;
	for (let i = 0; i < q.length && i < data.results.length; i++) {
		const r = data.results[i];
		if (r.status === 'applied') { applied++; appliedIds.push(q[i].mutationId); }
		else if (r.status === 'skipped') { skipped++; appliedIds.push(q[i].mutationId); }
	}
	// drop processed (both applied and skipped) - skipped already reverted by caller via serverRecord
	dequeueByIds(wid, appliedIds);
	// ponytail: expose for support
	if (typeof window !== 'undefined') (window as unknown as Record<string, unknown>).__syncLastResults = data.results;
	console.log(`offline sync: ${applied} applied, ${skipped} skipped`);
	return { applied, skipped, results: data.results };
}

// Same-tab notification that the queue changed (storage events only fire in
// other tabs). The [wid] layout listens for this to update its banner.
function notifyChanged() {
	if (typeof window === 'undefined') return;
	window.dispatchEvent(new Event('offline-queue-changed'));
}

if (typeof window !== 'undefined') {
	(window as unknown as Record<string, unknown>).__offlineQueue = { getQueue, enqueue, syncQueue, queuedCount, clearQueue, cacheGuests, loadCachedGuests };
}
