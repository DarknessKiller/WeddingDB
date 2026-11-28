import { apiFetch } from '#lib/api/client.ts';
import { addToast } from '#lib/stores/index.ts';
import { get } from 'svelte/store';
import { applyGuestResponse, clearGuestCaches, guestMap } from '#lib/stores/guestEvents.ts';
import type { GuestResponse } from '#lib/api/guests.ts';
import { weddingId as currentWeddingId } from '#lib/stores/weddingId.ts';

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
// Kiosk only: public rows must not overwrite the staff cache (offline_cache_{wid}).
function ckey(wid: string) { return `kiosk_cache_${wid}`; }

export function getQueue(wid: string): QueuedMutation[] {
	if (typeof window === 'undefined' || !wid) return [];
	try { return JSON.parse(localStorage.getItem(qkey(wid)) || '[]'); } catch { return []; }
}
function setQueue(wid: string, q: QueuedMutation[]) {
	localStorage.setItem(qkey(wid), JSON.stringify(q));
}

const QUEUE_CAP = 500;

// A full queue refuses the edit: dropping the oldest could lose a create and,
// with it, every later edit to that guest. Throws before any optimistic patch.
export function enqueue(wid: string, m: QueuedMutation): Promise<void> {
	if (getQueue(wid).length >= QUEUE_CAP) {
		throw new Error(`Offline queue full (${QUEUE_CAP} edits): reconnect to sync before making more changes`);
	}
	// The guest's updatedAt as this device last saw it, read before the optimistic patch. Null for a create.
	m.baseUpdatedAt ??= get(guestMap).get(m.guestId)?.updatedAt?.toISOString() ?? null;
	const write = () => {
		const q = getQueue(wid);
		q.push(m);
		setQueue(wid, q);
		// Without Web Locks another tab can write between the read above and this
		// write. Re-read and merge by mutationId so its edit survives ours.
		const persisted = getQueue(wid);
		if (!persisted.some((x) => x.mutationId === m.mutationId)) {
			persisted.push(m);
			setQueue(wid, persisted);
		}
		notifyChanged();
		addToast(`Queued offline (${Math.max(q.length, persisted.length)})`, 'info');
	};
	// Persist synchronously first: a reload right after the edit must keep it.
	write();
	// Then merge again under the lock, serialized across tabs; the fallback merge
	// above only rescues writes that land after ours.
	if (typeof navigator !== 'undefined' && navigator.locks) {
		return navigator.locks.request(qkey(wid), () => {
			const q = getQueue(wid);
			if (q.some((x) => x.mutationId === m.mutationId)) return;
			q.push(m);
			setQueue(wid, q);
			notifyChanged();
		}).catch(() => {});
	}
	return Promise.resolve();
}

export function dequeueByIds(wid: string, ids: string[]): void | Promise<void> {
	const s = new Set(ids);
	const write = () => {
		setQueue(wid, getQueue(wid).filter(x => !s.has(x.mutationId)));
		// A concurrent tab may have written the removed mutations back; drop them again.
		const after = getQueue(wid);
		const left = after.filter((x) => !s.has(x.mutationId));
		if (left.length !== after.length) setQueue(wid, left);
		notifyChanged();
	};
	// Same lock as enqueue: an unlocked read-modify-write could land after
	// another tab's locked enqueue and delete its fresh mutation.
	if (typeof navigator !== 'undefined' && navigator.locks) {
		return navigator.locks.request(qkey(wid), write);
	}
	write();
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

// The backend caps request bodies at 1 MiB, so the queue goes up in chunks.
const SYNC_CHUNK = 100;

export type SyncSummary = { applied: number; skipped: number; dropped: number };

// One drain per wedding at a time. The mount, the SSE reconnect, `online` and a
// manual retry can all fire together, and two overlapping drains replay the same
// mutations twice.
const inFlight = new Map<string, Promise<SyncSummary>>();

export function syncQueue(wid: string): Promise<SyncSummary> {
	const running = inFlight.get(wid);
	if (running) return running;
	const run = drainQueue(wid).finally(() => { inFlight.delete(wid); });
	inFlight.set(wid, run);
	return run;
}

async function drainQueue(wid: string): Promise<SyncSummary> {
	const q = getQueue(wid);
	let applied = 0, skipped = 0, dropped = 0;
	let retryReason = '';
	for (let start = 0; start < q.length; start += SYNC_CHUNK) {
		const chunk = q.slice(start, start + SYNC_CHUNK);
		const res = await apiFetch(`/api/weddings/${wid}/guests/sync`, {
			method: 'POST',
			body: JSON.stringify({ mutations: chunk })
		});
		if (!res.ok) {
			const t = await res.text().catch(() => '');
			// A rejected body is rejected every time: keeping it would block the
			// queue for good. Drop the chunk and say so.
			// 403 means the queue belongs to another account (or lacks rights):
			// never drop it silently; it must reach the owner-change prompt.
			if (res.status >= 400 && res.status < 500 && res.status !== 401 && res.status !== 403 && res.status !== 429) {
				await dequeueByIds(wid, chunk.map((m) => m.mutationId));
				addToast(`${chunk.length} offline edit${chunk.length === 1 ? '' : 's'} discarded: rejected by the server`, 'error');
				continue;
			}
			throw new Error(`sync failed ${res.status} ${t}`);
		}
		const data = await res.json() as { results: { guestId: string; status: string; serverRecord?: GuestResponse; reason?: string; retryable?: boolean }[] };
		const doneIds: string[] = [];
		const droppedReasons: string[] = [];
		for (let i = 0; i < chunk.length && i < data.results.length; i++) {
			const r = data.results[i];
			if (r.status === 'applied') { applied++; doneIds.push(chunk[i].mutationId); }
			else if (r.status === 'skipped') {
				// No column won: show the server's row instead of the optimistic one.
				skipped++; doneIds.push(chunk[i].mutationId);
				// The user may have switched wedding mid-drain; its rows must not land in the new one.
				if (r.serverRecord && wid === get(currentWeddingId)) applyGuestResponse(r.serverRecord);
			} else if (r.status === 'failed' && r.retryable === false) {
				// Validation or business rejection: re-sending it can never succeed,
				// so it must leave the queue for the rest to drain.
				dropped++; doneIds.push(chunk[i].mutationId);
				if (r.reason) droppedReasons.push(r.reason);
			} else if (r.status === 'failed') {
				retryReason ||= r.reason || 'the server refused it for now';
			}
		}
		await dequeueByIds(wid, doneIds);
		if (droppedReasons.length) {
			addToast(`${droppedReasons.length} offline edit${droppedReasons.length === 1 ? '' : 's'} discarded: ${droppedReasons[0]}`, 'error');
		}
	}
	if (skipped) addToast(`${skipped} offline edit${skipped === 1 ? '' : 's'} reverted: newer changes won`, 'info');
	// A retryable failure stays queued; say why, or the edit looks lost.
	if (retryReason) addToast(`Offline edits still queued: ${retryReason}`, 'error');
	return { applied, skipped, dropped };
}

// Owner is the user id (JWT `sub`), never the email: localStorage on a shared
// device must not hold another user's address. The label is the masked email.
const OWNER_KEY = 'offline_owner';
const OWNER_LABEL_KEY = 'offline_owner_label';

/** `jane@x.io` → `j***@x.io`. Shown when asking to discard another user's edits. */
export function maskEmail(email: string): string {
	const at = email.lastIndexOf('@');
	return at > 0 ? `${email[0]}***${email.slice(at)}` : '***';
}

/**
 * Queued edits follow their owner (Offline sync invariant 9). When a different
 * account logs in, replay every queue with the new session first, then ask
 * before discarding what is left. Call after login, once a token is set.
 */
export async function claimQueues(
	ownerId: string,
	email: string,
	confirmDiscard: (left: number, prevOwner: string) => boolean
): Promise<void> {
	if (typeof window === 'undefined' || !ownerId) return;
	const prev = localStorage.getItem(OWNER_KEY);
	if (prev && prev !== ownerId) {
		for (const wid of queuedWeddings()) {
			try { await syncQueue(wid); } catch {}
		}
		const left = queuedWeddings();
		const count = left.reduce((n, wid) => n + getQueue(wid).length, 0);
		const prevLabel = localStorage.getItem(OWNER_LABEL_KEY) || 'the previous account';
		// Declined: keep the edits and the old owner, so the next login asks again.
		if (count && !confirmDiscard(count, prevLabel)) return;
		for (const wid of left) localStorage.removeItem(qkey(wid));
		// The previous account's cached guest lists hold emails and angbao amounts.
		clearGuestCaches();
		notifyChanged();
	}
	localStorage.setItem(OWNER_KEY, ownerId);
	localStorage.setItem(OWNER_LABEL_KEY, maskEmail(email));
}

function queuedWeddings(): string[] {
	const wids: string[] = [];
	for (let i = 0; i < localStorage.length; i++) {
		const k = localStorage.key(i);
		if (k?.startsWith('offline_queue_')) wids.push(k.slice('offline_queue_'.length));
	}
	return wids.filter((wid) => getQueue(wid).length);
}

// Same-tab notification that the queue changed (storage events only fire in
// other tabs). The [wid] layout listens for this to update its banner.
function notifyChanged() {
	if (typeof window === 'undefined') return;
	window.dispatchEvent(new Event('offline-queue-changed'));
}
