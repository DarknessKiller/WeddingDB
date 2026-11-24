import { apiFetch, isNetworkError } from './client';
import { get } from 'svelte/store';
import { weddingId } from '#lib/stores/weddingId.ts';
import { applyGuestResponse, guestMap, removeGuest, upsertGuest } from '#lib/stores/guestEvents.ts';
import { enqueue, type SyncOp } from '#lib/offline/queue.ts';
import { applyGuestPatch, toGuest, type GuestFields } from '#lib/utils/guestTransform.ts';

function isOffline() {
	return typeof navigator !== 'undefined' && !navigator.onLine;
}
function genId(): string {
	if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
	// randomUUID needs a secure context (plain-HTTP LAN); the backend still wants an RFC 4122 v4 UUID.
	const b = crypto.getRandomValues(new Uint8Array(16));
	b[6] = (b[6] & 0x0f) | 0x40;
	b[8] = (b[8] & 0x3f) | 0x80;
	const h = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
	return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}
function nowIso() { return new Date().toISOString(); }
function optimisticPatch(op: SyncOp, guestId: string, data: GuestFields = {}) {
	if (op === 'delete') return removeGuest(guestId);
	const g = get(guestMap).get(guestId);
	if (!g) return;
	const now = new Date();
	if (op === 'update') upsertGuest({ ...applyGuestPatch(g, data), updatedAt: now });
	else if (op === 'checkout') upsertGuest({ ...g, checkedIn: false, checkedInAt: undefined, updatedAt: now });
	else if (op === 'checkin') {
		// Check-in notes are appended server-side, so mirror the append here.
		const added = data.notes?.trim();
		const notes = added ? (g.notes ? `${g.notes}\n${added}` : added) : undefined;
		upsertGuest({ ...applyGuestPatch(g, { ...data, notes }), checkedIn: true, checkedInAt: now, rsvp: 'confirmed', updatedAt: now });
	}
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
	/** Client-minted UUID: a retried POST returns the existing guest instead of a duplicate. */
	id?: string;
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
		const page = await listGuests(weddingId, { limit: 100, cursor });
		all.push(...page.guests);
		cursor = page.nextCursor ?? undefined;
	} while (cursor);
	return all;
}

// errorMessage keeps the server's title (seat overlap, cap) so a rejected save
// says why, not just the status.
async function errorMessage(res: Response, fallback: string): Promise<string> {
	const err = await res.json().catch(() => null);
	return err?.title || `${fallback}: ${res.status}`;
}

export async function getGuest(weddingId: string, guestId: string): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`);
	if (!res.ok) throw new Error(`Failed to get guest: ${res.status}`);
	return res.json();
}

export async function createGuest(weddingId: string, data: GuestCreateData): Promise<GuestResponse> {
	// One id for the POST and its queued fallback: if the POST landed but the
	// response was lost, the replay merges into that guest instead of duplicating it.
	const id = genId();
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests`, { method: 'POST', body: JSON.stringify({ ...data, id }) });
		if (!res.ok) throw new Error(await errorMessage(res, 'Failed to create guest'));
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		if (isNetworkError(e)) {
			enqueue(weddingId, { mutationId: genId(), op: 'create', guestId: id, clientUpdatedAt: nowIso(), payload: data as unknown as Record<string, unknown> });
			const now = nowIso();
			const resp: GuestResponse = { id, weddingId, name: data.name, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: data.tableId ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now };
			// Local clock, not a server timestamp: insert without seeding the
			// staleness watermark, or a fast clock would discard real server rows.
			upsertGuest(toGuest(resp));
			return resp;
		}
		throw e;
	}
}

/** Offline echo of an update, merged from the optimistic store entry so fields
 * the patch did not carry keep their current values. */
function offlineGuestResponse(weddingId: string, guestId: string): GuestResponse | null {
	const g = get(guestMap).get(guestId);
	if (!g) return null;
	return {
		id: g.id, weddingId, name: g.name, pinyin: g.pinyin, email: g.email ?? '',
		pax: g.pax, rsvp: g.rsvp, isVip: g.isVip, notes: g.notes,
		dietary: g.dietaryRequirements, tableId: g.tableId, seatNum: g.seatNumber,
		checkedInAt: g.checkedInAt ? g.checkedInAt.toISOString() : null,
		angbaoAmt: g.angbaoAmount ?? null, giftItem: g.giftItem ?? null,
		createdAt: g.createdAt.toISOString(), updatedAt: g.updatedAt?.toISOString() ?? nowIso(),
	};
}

export async function updateGuest(weddingId: string, guestId: string, data: Partial<GuestCreateData>): Promise<GuestResponse | null> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt: nowIso(), payload: data as unknown as Record<string, unknown> }); optimisticPatch('update', guestId, data); };
	if (isOffline()) { doQueue(); return offlineGuestResponse(weddingId, guestId); }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'PUT', body: JSON.stringify(data) });
		if (!res.ok) throw new Error(await errorMessage(res, 'Failed to update guest'));
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return offlineGuestResponse(weddingId, guestId); }
		throw e;
	}
}

export async function deleteGuest(weddingId: string, guestId: string): Promise<void> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'delete', guestId, clientUpdatedAt: nowIso() }); optimisticPatch('delete', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'DELETE' });
		if (!res.ok) throw new Error(await errorMessage(res, 'Failed to delete guest'));
		// Don't wait for the SSE echo: the row is gone from every store now.
		optimisticPatch('delete', guestId);
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
	// Stamped before the online attempt: if that POST landed and only the response
	// was lost, the replay is not newer than the server's check-in, so the server
	// skips it instead of appending the notes twice.
	const clientUpdatedAt = nowIso();
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'checkin', guestId, clientUpdatedAt, payload: payload ?? null }); optimisticPatch('checkin', guestId, payload); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkin`, { method: 'POST', body: JSON.stringify({ notes: trimmed }) });
		if (res.status === 409) { const err = await res.json().catch(() => ({ title: 'Guest already checked in' })); throw new ConflictError(err.title || 'Guest already checked in'); }
		if (!res.ok) throw new Error(`Failed to check in guest: ${res.status}`);
		optimisticPatch('checkin', guestId, payload);
	} catch (e) {
		if (e instanceof ConflictError) throw e;
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function checkOutGuest(weddingId: string, guestId: string): Promise<void> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'checkout', guestId, clientUpdatedAt: nowIso() }); optimisticPatch('checkout', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkout`, { method: 'POST' });
		if (!res.ok) throw new Error(`Failed to check out guest: ${res.status}`);
		optimisticPatch('checkout', guestId);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function assignSeat(weddingId: string, guestId: string, tableId: string, seatNum: number): Promise<void> {
	const payload = { tableId, seatNum };
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt: nowIso(), payload: payload as unknown as Record<string, unknown> }); optimisticPatch('update', guestId, payload); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, { method: 'POST', body: JSON.stringify(payload) });
		if (!res.ok) throw new Error(await errorMessage(res, 'Failed to assign seat'));
		// Seating maps read the shared guest store; patch it instead of relying on
		// the SSE echo, so a save shows its new occupancy immediately.
		optimisticPatch('update', guestId, payload);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function unassignSeat(weddingId: string, guestId: string): Promise<void> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt: nowIso(), payload: { tableId: null, seatNum: null } }); optimisticPatch('update', guestId, { tableId: null, seatNum: null }); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, { method: 'DELETE' });
		if (!res.ok) throw new Error(await errorMessage(res, 'Failed to unassign seat'));
		optimisticPatch('update', guestId, { tableId: null, seatNum: null });
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}
export async function bulkImportGuests(guests: GuestImportData[]): Promise<{ imported: number }> {
  if (isOffline()) throw new Error('Bulk import needs connection');
  const res = await apiFetch(`/api/weddings/${get(weddingId)}/guests/import`, {
    method: 'POST',
    body: JSON.stringify({ guests }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ title: 'Import failed' }));
    throw new Error(err.title || 'Import failed');
  }
  return res.json();
}
