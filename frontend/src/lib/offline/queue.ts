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

export function queuedCount(wid: string): number {
	return getQueue(wid).length;
}

/** Weddings whose unsynced edits are still queued on this device. */
export function weddingsWithQueuedEdits(): string[] {
	if (typeof window === 'undefined') return [];
	const ids: string[] = [];
	for (let i = 0; i < localStorage.length; i++) {
		const key = localStorage.key(i) ?? '';
		if (!key.startsWith(QUEUE_PREFIX)) continue;
		const wid = key.slice(QUEUE_PREFIX.length);
		if (queuedCount(wid) > 0) ids.push(wid);
	}
	return ids;
}

/**
 * Tries to send every wedding's queued edits, then reports how many are left.
 * Call before anything that deletes the queue or strands it (logout).
 */
export async function flushAllQueues(): Promise<number> {
	for (const wid of weddingsWithQueuedEdits()) await syncQueue(wid).catch(() => {});
	return weddingsWithQueuedEdits().reduce((n, wid) => n + queuedCount(wid), 0);
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
	// inFlight only covers this tab. A Web Lock also holds off other tabs of
	// this browser; runSync re-reads the queue inside it, so a second tab
	// sends only what the first one left.
	const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
	const sync = locks ? locks.request(`weddingdb-sync-${wid}`, () => runSync(wid)) : runSync(wid);
	const run = sync.finally(() => {
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
		body: JSON.stringify({ mutations: q }),
		// A full queue replays many edits; a timeout keeps them queued for the next sync.
		signal: AbortSignal.timeout(60_000)
	});
	if (!res.ok) {
		const t = await res.text().catch(() => '');
		throw new Error(`sync failed ${res.status} ${t}`);
	}
	const data = await res.json() as { results: { guestId: string; status: string; serverRecord?: unknown; reason?: string; retryable?: boolean }[] };
	const appliedIds: string[] = [];
	const failedIds: string[] = [];
	let applied = 0, skipped = 0;
	for (let i = 0; i < q.length && i < data.results.length; i++) {
		const r = data.results[i];
		if (r.status === 'applied') { applied++; appliedIds.push(q[i].mutationId); }
		// Skipped but already on the server: an online write that timed out after
		// landing, then queued and replayed. Nothing was lost.
		else if (r.status === 'skipped' && alreadyApplied(q[i], r.serverRecord)) { applied++; appliedIds.push(q[i].mutationId); }
		else if (r.status === 'skipped') { skipped++; appliedIds.push(q[i].mutationId); }
		// A retryable failure (server-side outage) stays queued without spending
		// an attempt; only rejections of the edit itself count toward the limit.
		else if (r.status === 'failed' && !r.retryable) failedIds.push(q[i].mutationId);
	}
	// drop processed (both applied and skipped); the server already holds the newer record
	dequeueByIds(wid, appliedIds);
	const dropped = bumpAttempts(wid, failedIds);
	// Staff must know when offline edits did not land, so they can re-enter them.
	if (skipped) addToast(`${skipped} offline edit(s) were older than the server copy and were not applied`, 'error');
	if (dropped) addToast(`${dropped} offline edit(s) failed ${MAX_ATTEMPTS} times and were discarded`, 'error');
	return { applied, skipped, results: data.results };
}
/** True when the server record already holds what the queued mutation would write. */
function alreadyApplied(m: QueuedMutation, record: unknown): boolean {
	if (!record || typeof record !== 'object') return false;
	const r = record as Record<string, unknown>;
	if (m.op === 'checkin') return r.checkedInAt != null;
	if (m.op === 'checkout') return r.checkedInAt == null;
	if (m.op === 'delete' || !m.payload) return false;
	return Object.entries(m.payload).every(([k, v]) =>
		k === 'baseUpdatedAt' || JSON.stringify(r[k] ?? null) === JSON.stringify(v ?? null));
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
