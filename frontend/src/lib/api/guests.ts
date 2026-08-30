import { apiFetch } from './client';
import { get } from 'svelte/store';
import { weddingId } from '$lib/stores/weddingId';
import { applyGuestResponse } from '$lib/stores/guestEvents';
import { enqueue } from '$lib/offline/queue';
import { guestList, guestMap } from '$lib/stores/guestEvents';

function isOffline() {
	return typeof navigator !== 'undefined' && !navigator.onLine;
}
function isNetworkError(e: unknown) {
	return e instanceof TypeError && String((e as Error).message || '').toLowerCase().includes('fetch');
}
function genId(): string {
	try { return crypto.randomUUID(); } catch { return `${Date.now()}-${Math.random().toString(36).slice(2)}`; }
}
function nowIso() { return new Date().toISOString(); }
function optimisticPatch(op: string, guestId: string, data?: Record<string, unknown>) {
	try {
		if (op === 'create' && data) {
			const g = { id: guestId, name: (data.name as string) || '', phone: (data.phone as string) || '', email: (data.email as string) || '', pax: (data.pax as number) || 1, rsvp: (data.rsvp as string) || 'no_response', isVip: !!data.isVip, notes: (data.notes as string) || '', dietaryRequirements: (data.dietary as string[]) || [], tableId: (data.tableId as string | null) || null, seatNumber: (data.seatNum as number | null) ?? null, checkedIn: false, createdAt: new Date() } as unknown as import('$lib/types').Guest;
			guestList.update(l => [...l, g]); guestMap.update(m => { const n = new Map(m); n.set(guestId, g); return n; });
		} else if (op === 'update' && data) {
			guestList.update(l => l.map(x => x.id === guestId ? { ...x, ...{ name: data.name, phone: data.phone, email: data.email, pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietaryRequirements: data.dietary, tableId: data.tableId ?? x.tableId, seatNumber: data.seatNum ?? x.seatNumber, angbaoAmount: data.angbaoAmt ?? x.angbaoAmount, giftItem: data.giftItem ?? x.giftItem } } as unknown as import('$lib/types').Guest : x));
		} else if (op === 'delete') {
			guestList.update(l => l.filter(x => x.id !== guestId)); guestMap.update(m => { const n = new Map(m); n.delete(guestId); return n; });
		} else if (op === 'checkin') {
			guestList.update(l => l.map(x => x.id === guestId ? { ...x, checkedIn: true, checkedInAt: new Date() } as unknown as import('$lib/types').Guest : x));
		} else if (op === 'checkout') {
			guestList.update(l => l.map(x => x.id === guestId ? { ...x, checkedIn: false, checkedInAt: undefined } as unknown as import('$lib/types').Guest : x));
		}
	} catch {}
}

export class ConflictError extends Error {
	constructor(message: string) {
		super(message);
		this.name = 'ConflictError';
	}
}

export interface GuestImportData {
  name: string;
  phone?: string;
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
	phone: string;
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
	phone: string;
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

export async function getGuest(weddingId: string, guestId: string): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`);
	if (!res.ok) throw new Error(`Failed to get guest: ${res.status}`);
	return res.json();
}

export async function createGuest(weddingId: string, data: GuestCreateData): Promise<GuestResponse> {
	const doQueue = (id: string) => {
		enqueue(weddingId, { mutationId: genId(), op: 'create', guestId: id, clientUpdatedAt: nowIso(), payload: data as unknown as Record<string, unknown> });
	};
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests`, { method: 'POST', body: JSON.stringify(data) });
		if (!res.ok) throw new Error(`Failed to create guest: ${res.status}`);
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		if (isNetworkError(e)) {
			const id = genId();
			doQueue(id); optimisticPatch('create', id, data as unknown as Record<string, unknown>);
			const now = nowIso();
			return { id, weddingId, name: data.name, phone: data.phone, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now };
		}
		throw e;
	}
}

export async function updateGuest(weddingId: string, guestId: string, data: GuestCreateData): Promise<GuestResponse> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt: nowIso(), payload: data as unknown as Record<string, unknown> }); optimisticPatch('update', guestId, data as unknown as Record<string, unknown>); };
	if (isOffline()) { doQueue(); const now = nowIso(); return { id: guestId, weddingId, name: data.name, phone: data.phone, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now }; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'PUT', body: JSON.stringify(data) });
		if (!res.ok) throw new Error(`Failed to update guest: ${res.status}`);
		const resp: GuestResponse = await res.json();
		applyGuestResponse(resp);
		return resp;
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); const now = nowIso(); return { id: guestId, weddingId, name: data.name, phone: data.phone, email: data.email || '', pax: data.pax, rsvp: data.rsvp, isVip: data.isVip, notes: data.notes, dietary: data.dietary, tableId: (data.tableId as string | null) ?? null, seatNum: data.seatNum ?? null, checkedInAt: null, angbaoAmt: data.angbaoAmt ?? null, giftItem: data.giftItem ?? null, createdAt: now, updatedAt: now }; }
		throw e;
	}
}

export async function deleteGuest(weddingId: string, guestId: string): Promise<void> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'delete', guestId, clientUpdatedAt: nowIso() }); optimisticPatch('delete', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, { method: 'DELETE' });
		if (!res.ok) throw new Error(`Failed to delete guest: ${res.status}`);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function checkInGuest(weddingId: string, guestId: string, body?: { angbaoAmt?: number; giftItem?: string }): Promise<void> {
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'checkin', guestId, clientUpdatedAt: nowIso(), payload: body as unknown as Record<string, unknown> }); optimisticPatch('checkin', guestId); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkin`, { method: 'POST', body: body ? JSON.stringify(body) : undefined });
		if (res.status === 409) { const err = await res.json().catch(() => ({ title: 'Guest already checked in' })); throw new ConflictError(err.title || 'Guest already checked in'); }
		if (!res.ok) throw new Error(`Failed to check in guest: ${res.status}`);
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
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}

export async function assignSeat(weddingId: string, guestId: string, tableId: string, seatNum: number): Promise<void> {
	const payload = { tableId, seatNum };
	const doQueue = () => { enqueue(weddingId, { mutationId: genId(), op: 'update', guestId, clientUpdatedAt: nowIso(), payload: payload as unknown as Record<string, unknown> }); optimisticPatch('update', guestId, payload as unknown as Record<string, unknown>); };
	if (isOffline()) { doQueue(); return; }
	try {
		const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, { method: 'POST', body: JSON.stringify(payload) });
		if (!res.ok) throw new Error(`Failed to assign seat: ${res.status}`);
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
		if (!res.ok) throw new Error(`Failed to unassign seat: ${res.status}`);
	} catch (e) {
		if (isNetworkError(e)) { doQueue(); return; }
		throw e;
	}
}
export async function syncOfflineQueue(weddingId: string): Promise<{ applied: number; skipped: number }> {
	const { syncQueue } = await import('$lib/offline/queue');
	return syncQueue(weddingId);
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
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ title: 'Import failed' }));
    throw new Error(err.title || 'Import failed');
  }
  return res.json();
}
