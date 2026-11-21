import { apiFetch } from './client';
import { get } from 'svelte/store';
import { weddingId } from '#lib/stores/weddingId.ts';
import { applyGuestResponse } from '#lib/stores/guestEvents.ts';
import { enqueue, getQueue } from '#lib/offline/queue.ts';
import type { Guest } from '#lib/types/index.ts';
import { guestList, cacheGuestList } from '#lib/stores/guestEvents.ts';
import { GUEST_NOTES_MAXLEN } from '#lib/utils/limits.ts';

function isOffline() {
	return typeof navigator !== 'undefined' && !navigator.onLine;
}
// Matches the server's MaxGuestNotesLen (characters), so a note is never
// refused after the form was filled in.
export const MAX_GUEST_NOTES_LEN = GUEST_NOTES_MAXLEN;

// noteRoom is how many characters a check-in note can add to existing notes
// (joined with a newline) without passing MAX_GUEST_NOTES_LEN.
export function noteRoom(existing: string): number {
	const used = [...existing].length + (existing ? 1 : 0);
	return Math.max(0, MAX_GUEST_NOTES_LEN - used);
}

export function isNetworkError(e: unknown) {
	if (isOffline()) return true;
	// Chrome/Firefox say "Failed to fetch"/"NetworkError"; Safari says "Load failed".
	if (e instanceof DOMException && e.name === 'TimeoutError') return true;
	return e instanceof TypeError && /failed to fetch|networkerror|load failed/i.test((e as Error).message || '');
}
// genId makes a random 16-byte id in the server's base64url UUID form, so an
// offline-created guest keeps the id SSE echoes back (no duplicate row).
// getRandomValues also works outside HTTPS, unlike crypto.randomUUID.
function genId(): string {
	const b = crypto.getRandomValues(new Uint8Array(16));
	return btoa(String.fromCharCode(...b)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
// Capture once per user action and reuse for the queued op: a retry after a lost
// response must not look newer than the original under server last-write-wins.
function nowIso() { return new Date().toISOString(); }
function optimisticPatch(op: string, guestId: string, data?: Record<string, unknown>) {
	try {
		guestList.update(l => patchGuests(l, op, guestId, data));
		// Persist patched roster: offline reload must show queued edits,
		// or staff re-create a guest that is still waiting to sync.
		cacheGuestList();
	} catch {}
}

/**
 * Server roster with this device's still-queued edits laid over it. A reseed
 * from the server must not visually revert (or uncache) an unsynced edit.
 */
export function withQueuedEdits(wid: string, guests: Guest[]): Guest[] {
	return getQueue(wid).reduce((l, m) => patchGuests(l, m.op, m.guestId, m.payload ?? undefined), guests);
}

function patchGuests(l: Guest[], op: string, guestId: string, data?: Record<string, unknown>): Guest[] {
	if (op === 'create' && data) {
		const now = new Date();
		const g = { id: guestId, name: (data.name as string) || '', email: (data.email as string) || '', pax: (data.pax as number) || 1, rsvp: (data.rsvp as string) || 'no_response', isVip: !!data.isVip, notes: (data.notes as string) || '', dietaryRequirements: (data.dietary as string[]) || [], tableId: (data.tableId as string | null) || null, seatNumber: (data.seatNum as number | null) ?? null, checkedIn: false, createdAt: now, updatedAt: now } as unknown as import('#lib/types/index.ts').Guest;
		return l.some(x => x.id === guestId) ? l : [...l, g];		} else if (op === 'update' && data) {
		// Explicit null (or "") in the patch overrides/clears the old value;
		// undefined keeps the old value (partial payloads like seat assign).
		return l.map(x => x.id === guestId ? {
			...x,
			name: data.name !== undefined ? data.name : x.name,
			email: data.email !== undefined ? data.email : x.email,
			pax: data.pax !== undefined ? data.pax : x.pax,
			rsvp: data.rsvp !== undefined ? data.rsvp : x.rsvp,
			isVip: data.isVip !== undefined ? data.isVip : x.isVip,
			notes: data.notes !== undefined ? data.notes : x.notes,
			dietaryRequirements: data.dietary !== undefined ? data.dietary : x.dietaryRequirements,
			tableId: data.tableId !== undefined ? ((data.tableId as string | null) || null) : x.tableId,
			seatNumber: data.seatNum !== undefined ? ((data.seatNum as number | null) ?? null) : x.seatNumber,
			angbaoAmount: data.angbaoAmt !== undefined ? ((data.angbaoAmt as number | null) ?? undefined) : x.angbaoAmount,
			giftItem: data.giftItem !== undefined ? ((data.giftItem as string | null) ?? undefined) : x.giftItem,
			updatedAt: new Date()
		} as unknown as import('#lib/types/index.ts').Guest : x);
	} else if (op === 'delete') {
		return l.filter(x => x.id !== guestId);		} else if (op === 'checkin') {
		// Mirror server replay: a checkin the server already holds (response lost)
		// keeps its time, money and gift, and its note is not appended twice.
		const now = new Date();
		const addedNotes = typeof data?.notes === 'string' ? data.notes.trim() : '';
		return l.map(x => {
			if (x.id !== guestId) return x;
			const was = x.checkedIn;
			const hasNote = was && (x.notes ?? '').split('\n').includes(addedNotes);
			return {
				...x, checkedIn: true, checkedInAt: was ? x.checkedInAt : now, rsvp: 'confirmed' as const,
				notes: addedNotes && !hasNote ? (x.notes ? `${x.notes}\n${addedNotes}` : addedNotes) : x.notes,
				angbaoAmount: data?.angbaoAmt != null && (!was || x.angbaoAmount == null) ? (data.angbaoAmt as number) : x.angbaoAmount,
				giftItem: data?.giftItem != null && (!was || x.giftItem == null) ? (data.giftItem as string) : x.giftItem,
				updatedAt: now
			} as unknown as import('#lib/types/index.ts').Guest;
		});
	} else if (op === 'checkout') {
		return l.map(x => x.id === guestId ? { ...x, checkedIn: false, checkedInAt: undefined, updatedAt: new Date() } as unknown as import('#lib/types/index.ts').Guest : x);
	}
	return l;
}

// serverError keeps the server's reason (table full, seat taken, guest cap)
// for the toast, falling back to the status when the body has none.
async function serverError(res: Response, action: string): Promise<Error> {
	const err = await res.json().catch(() => ({}));
	return new Error(err.title || `Failed to ${action}: ${res.status}`);
}

export class ConflictError extends Error {
	constructor(message: string) {
		super(message);
		this.name = 'ConflictError';
	}
}

export interface GuestImportData {
  name: string;
  email?: string;
  pax?: number;
  rsvp?: string;
  isVip?: boolean;
  notes?: string;
  dietary?: string[];
  table?: string;
  seat?: number;
  tableId?: string;
  seatNum?: number;
  /** Spreadsheet row the guest came from; import errors name it. */
  row?: number;
}

export interface GuestResponse {
	id: string;
	weddingId: string;
	name: string;
	pinyin?: string;
	email: string;
	pax: number;
	rsvp: string;
	isVip: boolean;
	notes: string;
	dietary: string[];
	tableId: string | null;
	seatNum: number | null;
	checkedInAt: string | null;
	angbaoAmt: number | null;
	giftItem: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface GuestCreateData {
	name: string;
	email?: string;
	pax: number;
	rsvp: string;
	isVip: boolean;
	notes: string;
	dietary: string[];
	tableId?: string | null;
	seatNum?: number | null;
	angbaoAmt?: number | null;
	giftItem?: string | null;
	/** Server updatedAt the editor loaded; the server answers 409 if the guest changed since. */
	baseUpdatedAt?: string;
}

export async function listGuests(weddingId: string, opts?: { limit?: number; cursor?: string }): Promise<{ guests: GuestResponse[]; total: number; nextCursor: string | null }> {
	const params = new URLSearchParams();
	if (opts?.limit) params.set('limit', String(opts.limit));
	if (opts?.cursor) params.set('cursor', opts.cursor);
	const qs = params.toString();
	const res = await apiFetch(`/api/weddings/${weddingId}/guests${qs ? '?' + qs : ''}`);
	if (!res.ok) throw new Error(`Failed to list guests: ${res.status}`);
	return res.json();
}

export async function fetchAllGuests(weddingId: string): Promise<GuestResponse[]> {
	const all: GuestResponse[] = [];
	let cursor: string | undefined;
	do {
		// The server caps a page at 500: 1,000 guests take two requests, not ten.
		const page = await listGuests(weddingId, { limit: 500, cursor });
		all.push(...page.guests);
		cursor = page.nextCursor ?? undefined;
	} while (cursor);
	return all;
}

export async function getGuest(weddingId: string, guestId: string): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`);
	if (!res.ok) throw new Error(`Failed to get guest: ${res.status}`);
	return res.json();
}

export async function createGuest(weddingId: string, data: GuestCreateData): Promise<GuestResponse> {
	const clientUpdatedAt = nowIso();
	const wasOffline = isOffline();
	const doQueue = (id: string) => {
		enqueue(weddingId, { mutationId: genId(), op: 'create', guestId: id, clientUpdatedAt, payload: data as unknown as Record<string, unknown> });
	};
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests`, { method: 'POST', body: JSON.stringify(data) });
		if (!res.ok) throw await serverError(res, 'create guest');
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		// Online POST assigns a server id, so a create whose response was lost cannot be
		// replayed safely. Queue only when the request never left the device.
		if (wasOffline && isNetworkError(e)) {
			const id = genId();
			doQueue(id); optimisticPatch('create', id, data as unknown as Record<string, unknown>);
			const now = nowIso();
			return { id, weddingId, name: data.name, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now };
		}
		throw e;
	}
}

export async function updateGuest(weddingId: string, guestId: string, data: GuestCreateData): Promise<GuestResponse> {
	const clientUpdatedAt = nowIso();
	const doQueue = () => {
		// The base lets the server refuse an edit made on a stale copy, as the
		// online 409 does. Drop it when an earlier queued edit of this guest is
		// still pending: that edit will move updatedAt, and it is our own.
		const ownPending = getQueue(weddingId).some(m => m.guestId === guestId);
		enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt, baseUpdatedAt: ownPending ? null : data.baseUpdatedAt ?? null, payload: data as unknown as Record<string, unknown> });
		optimisticPatch('update', guestId, data as unknown as Record<string, unknown>);
	};
	if (isOffline()) { doQueue(); const now = nowIso(); return { id: guestId, weddingId, name: data.name, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now }; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'PUT', body: JSON.stringify(data) });
		if (res.status === 409) { const err = await res.json().catch(() => ({})); throw new ConflictError(err.title || 'Someone else changed this guest. Reload to see their changes.'); }
		if (!res.ok) throw await serverError(res, 'update guest');
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); const now = nowIso(); return { id: guestId, weddingId, name: data.name, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now }; }
		throw e;
	}
}

export async function deleteGuest(weddingId: string, guestId: string): Promise<void> {
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'delete', guestId, clientUpdatedAt }); optimisticPatch('delete', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'DELETE' });
		if (!res.ok) throw new Error(`Failed to delete guest: ${res.status}`);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

// checkInGuest checks a guest in. Optional notes are appended to the guest's
// notes server-side, atomically with the check-in.
export async function checkInGuest(weddingId: string, guestId: string, notes?: string): Promise<void> {
	const trimmed = (notes ?? '').trim();
	const payload = trimmed ? { notes: trimmed } : undefined;
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'checkin', guestId, clientUpdatedAt, payload: payload ?? null }); optimisticPatch('checkin', guestId, payload); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkin`, { method: 'POST', body: JSON.stringify({ notes: trimmed }) });
		if (res.status === 409) { const err = await res.json().catch(() => ({ title: 'Guest already checked in' })); throw new ConflictError(err.title || 'Guest already checked in'); }
		if (!res.ok) throw new Error(`Failed to check in guest: ${res.status}`);
	} catch (e) {
		if (e instanceof ConflictError) throw e;
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function checkOutGuest(weddingId: string, guestId: string): Promise<void> {
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'checkout', guestId, clientUpdatedAt }); optimisticPatch('checkout', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkout`, { method: 'POST' });
		if (!res.ok) throw new Error(`Failed to check out guest: ${res.status}`);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function assignSeat(weddingId: string, guestId: string, tableId: string, seatNum: number): Promise<void> {
	const payload = { tableId, seatNum };
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt, payload: payload as unknown as Record<string, unknown> }); optimisticPatch('update', guestId, payload as unknown as Record<string, unknown>); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, { method: 'POST', body: JSON.stringify(payload) });
		if (!res.ok) throw await serverError(res, 'assign seat');
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function unassignSeat(weddingId: string, guestId: string): Promise<void> {
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt, payload: { tableId: null, seatNum: null } }); optimisticPatch('update', guestId, { tableId: null, seatNum: null }); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, { method: 'DELETE' });
		if (!res.ok) throw new Error(`Failed to unassign seat: ${res.status}`);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function searchGuests(weddingId: string, query: string): Promise<GuestResponse[]> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error(`Failed to search guests: ${res.status}`);
	return res.json();
}

export async function bulkImportGuests(guests: GuestImportData[]): Promise<{ imported: number }> {
  if (isOffline()) throw new Error('Bulk import needs connection');
  const res = await apiFetch(`/api/weddings/${get(weddingId)}/guests/import`, {
    method: 'POST',
    body: JSON.stringify({ guests }),
    signal: AbortSignal.timeout(120_000), // up to 1,000 rows in one transaction
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ title: 'Import failed' }));
    throw new Error(err.title || 'Import failed');
  }
  return res.json();
}
