import { apiFetch } from '$lib/api/client';

export type SyncOp = 'create' | 'update' | 'delete' | 'checkin' | 'checkout';

export interface QueuedMutation {
	mutationId: string;
	op: SyncOp;
	guestId: string;
	clientUpdatedAt: string;
	baseUpdatedAt?: string | null;
	payload?: Record<string, unknown> | null;
}

function qkey(wid: string) { return `offline_queue_${wid}`; }
function ckey(wid: string) { return `offline_cache_${wid}`; }

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

export function cacheGuests(wid: string, guests: unknown[]) {
	try { localStorage.setItem(ckey(wid), JSON.stringify(guests)); } catch {}
}
export function loadCachedGuests<T>(wid: string): T[] | null {
	try {
		const v = localStorage.getItem(ckey(wid));
		return v ? JSON.parse(v) as T[] : null;
	} catch { return null; }
}

export async function syncQueue(wid: string): Promise<{ applied: number; skipped: number; results: unknown[] }> {
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
