import { get } from 'svelte/store';
import { apiFetch } from '#lib/api/client.ts';
import { applyGuestResponse, removeGuest, guestList } from '#lib/stores/guestEvents.ts';
import { addToast } from '#lib/stores/index.ts';
import type { GuestResponse } from '#lib/api/guests.ts';
import type { Guest } from '#lib/types/index.ts';

export type SyncOp = 'create' | 'update' | 'delete' | 'checkin' | 'checkout';

export interface QueuedMutation {
	mutationId: string;
	op: SyncOp;
	guestId: string;
	clientUpdatedAt: string;
	baseUpdatedAt?: string | null;
	payload?: Record<string, unknown> | null;
	// previous is the row this mutation's optimistic patch replaced, kept so a
	// rejected change can be rolled back when the server sends no record.
	previous?: Guest | null;
}

function qkey(wid: string) { return `offline_queue_${wid}`; }
function ckey(wid: string) { return `offline_cache_${wid}`; }

export function getQueue(wid: string): QueuedMutation[] {
	if (typeof window === 'undefined' || !wid) return [];
	try { return JSON.parse(localStorage.getItem(qkey(wid)) || '[]'); } catch { return []; }
}
function setQueue(wid: string, q: QueuedMutation[]) {
	const trimmed = q.slice(0, 500);
	try {
		localStorage.setItem(qkey(wid), JSON.stringify(trimmed));
	} catch {
		// Storage quota: keep the newest changes so the most recent work survives.
		const newest = trimmed.slice(-250);
		localStorage.setItem(qkey(wid), JSON.stringify(newest));
		addToast('Device storage is full: older queued changes were dropped.', 'error');
	}
}

// Queue mutations are read-modify-write on localStorage shared by every tab.
// Run them under one lock so a sync finishing in another tab cannot write back
// the queue it read before this tab enqueued, silently dropping the new edit.
function withQueueLock<T>(wid: string, fn: () => T): Promise<T> {
	const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
	if (!locks) return Promise.resolve(fn());
	return locks.request(`offline-queue-${wid}`, () => fn());
}

export function enqueue(wid: string, m: QueuedMutation): Promise<void> {
	return withQueueLock(wid, () => {
		const q = getQueue(wid);
		if (q.length >= 500) {
			q.shift();
			addToast('Offline queue is full (500 changes): the oldest unsynced change was dropped. Reconnect to sync.', 'error');
		}
		const current = get(guestList).find(g => g.id === m.guestId);
		if (m.previous === undefined) {
			m.previous = current ? JSON.parse(JSON.stringify(current)) as Guest : null;
		}
		q.push(m);
		try {
			setQueue(wid, q);
		} catch {
			addToast('Could not save this change on the device: storage is full.', 'error');
		}
		notifyChanged();
	});
}

export function dequeueByIds(wid: string, ids: string[]): Promise<void> {
	return withQueueLock(wid, () => {
		const s = new Set(ids);
		setQueue(wid, getQueue(wid).filter(x => !s.has(x.mutationId)));
		notifyChanged();
	});
}

export function clearQueue(wid: string): Promise<void> {
	return withQueueLock(wid, () => {
		localStorage.removeItem(qkey(wid));
		notifyChanged();
	});
}

function offlineKeys(): string[] {
	const keys: string[] = [];
	for (let i = 0; i < localStorage.length; i++) {
		const k = localStorage.key(i);
		if (k?.startsWith('offline_')) keys.push(k);
	}
	return keys;
}

/** Unsynced changes across every wedding in this browser. */
export function pendingChangeCount(): number {
	if (typeof window === 'undefined') return 0;
	return offlineKeys().filter(k => k.startsWith('offline_queue_')).reduce((n, k) => n + getQueue(k.slice('offline_queue_'.length)).length, 0);
}

/** Explicit logout: drop every queue and guest cache this browser holds. */
export function clearOfflineData() {
	if (typeof window === 'undefined') return;
	offlineKeys().forEach(k => localStorage.removeItem(k));
	notifyChanged();
}

// claimOfflineOwner drops another account's unsynced changes and cached guest
// list when a different user logs in on this shared device, then records the
// owner. Queued edits and guest PII must never pass to the next user.
export function claimOfflineOwner(userId: string) {
	if (typeof window === 'undefined' || !userId) return;
	const previous = localStorage.getItem('offline_owner');
	if (previous && previous !== userId) clearOfflineData();
	try {
		localStorage.setItem('offline_owner', userId);
	} catch {}
}

// rollbackOptimistic restores the row a rejected mutation replaced. The queue
// stores it as JSON, so dates are revived before the store sees them.
function rollbackOptimistic(m: QueuedMutation) {
	if (!m.previous) {
		if (m.op === 'create') removeGuest(m.guestId);
		return;
	}
	const previous = m.previous;
	const restored: Guest = {
		...previous,
		createdAt: new Date(previous.createdAt),
		updatedAt: new Date(previous.updatedAt ?? Date.now()),
		checkedInAt: previous.checkedInAt ? new Date(previous.checkedInAt) : undefined,
	};
	guestList.update(list => list.some(g => g.id === restored.id)
		? list.map(g => (g.id === restored.id ? restored : g))
		: [...list, restored]);
}

// Guest rows carry names, contacts and gift amounts; the offline cache must not
// outlive the day it was written.
const CACHE_TTL_MS = 12 * 60 * 60 * 1000;

export function cacheGuests(wid: string, guests: unknown[]) {
	try { localStorage.setItem(ckey(wid), JSON.stringify({ savedAt: Date.now(), guests })); } catch {}
}
export function loadCachedGuests<T>(wid: string): T[] | null {
	try {
		const v = localStorage.getItem(ckey(wid));
		if (!v) return null;
		const entry = JSON.parse(v) as { savedAt?: number; guests?: T[] };
		// Entries from before the TTL was added are dropped rather than trusted.
		if (!entry.savedAt || !entry.guests || Date.now() - entry.savedAt > CACHE_TTL_MS) {
			localStorage.removeItem(ckey(wid));
			return null;
		}
		return entry.guests;
	} catch { return null; }
}

type SyncSummary = { applied: number; skipped: number; results: unknown[] };
const inflight = new Map<string, Promise<SyncSummary>>();

// One sync per wedding at a time: a second caller joins the running one
// instead of sending the same queue again. Web Locks extend that across tabs.
export function syncQueue(wid: string): Promise<SyncSummary> {
	let p = inflight.get(wid);
	if (!p) {
		const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
		p = (locks ? locks.request(`offline-sync-${wid}`, () => runSync(wid)) : runSync(wid))
			.finally(() => inflight.delete(wid));
		inflight.set(wid, p);
	}
	return p;
}

async function runSync(wid: string): Promise<SyncSummary> {
	const q = getQueue(wid);
	if (!q.length) return { applied: 0, skipped: 0, results: [] };
	const res = await apiFetch(`/api/weddings/${wid}/guests/sync`, {
		method: 'POST',
		body: JSON.stringify({ mutations: q.map(({ previous, ...m }) => m) })
	});
	if (!res.ok) {
		const t = await res.text().catch(() => '');
		throw new Error(`sync failed ${res.status} ${t}`);
	}
	const data = await res.json() as { results: { guestId: string; status: string; serverRecord?: unknown; reason?: string; retryable?: boolean }[] };
	const appliedIds: string[] = [];
	const dropped: string[] = [], retrying: string[] = [];
	let applied = 0, skipped = 0;
	for (let i = 0; i < q.length && i < data.results.length; i++) {
		const r = data.results[i];
		if (r.status === 'applied') {
			applied++;
			appliedIds.push(q[i].mutationId);
		} else if (r.status === 'skipped') {
			skipped++;
			appliedIds.push(q[i].mutationId);
			// The server kept a newer value: undo the optimistic local edit.
			if (r.serverRecord) applyGuestResponse(r.serverRecord as GuestResponse);
			else removeGuest(q[i].guestId);
		} else if (r.status === 'failed' && r.retryable === false) {
			// Retrying cannot succeed (e.g. validation): drop it and show the server's value.
			dropped.push(r.reason || 'rejected');
			appliedIds.push(q[i].mutationId);
			if (r.serverRecord) applyGuestResponse(r.serverRecord as GuestResponse);
			else rollbackOptimistic(q[i]);
		} else if (r.status === 'failed') {
			retrying.push(r.reason || 'unknown error');
		}
	}
	await dequeueByIds(wid, appliedIds);
	if (skipped > 0) {
		addToast(`${skipped} offline change${skipped === 1 ? '' : 's'} rejected: the server had a newer version`, 'error');
	}
	if (dropped.length) {
		addToast(`${dropped.length} offline change${dropped.length === 1 ? '' : 's'} discarded by the server: ${dropped[0]}`, 'error');
	}
	if (retrying.length) {
		addToast(`${retrying.length} offline change${retrying.length === 1 ? '' : 's'} could not sync and will retry: ${retrying[0]}`, 'error');
	}
	return { applied, skipped, results: data.results };
}

// Same-tab notification that the queue changed (storage events only fire in
// other tabs). The [wid] layout listens for this to update its banner.
function notifyChanged() {
	if (typeof window === 'undefined') return;
	window.dispatchEvent(new Event('offline-queue-changed'));
}
