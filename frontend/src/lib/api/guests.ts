import { apiFetch } from './client';
import { get } from 'svelte/store';
import { weddingId } from '$lib/stores/weddingId';

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
	const res = await apiFetch(`/api/weddings/${weddingId}/guests`, {
		method: 'POST',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error(`Failed to create guest: ${res.status}`);
	return res.json();
}

export async function updateGuest(weddingId: string, guestId: string, data: GuestCreateData): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, {
		method: 'PUT',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error(`Failed to update guest: ${res.status}`);
	return res.json();
}

export async function deleteGuest(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error(`Failed to delete guest: ${res.status}`);
}

export async function checkInGuest(weddingId: string, guestId: string, body?: { angbaoAmt?: number; giftItem?: string }): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkin`, {
		method: 'POST',
		body: body ? JSON.stringify(body) : undefined,
	});
	if (res.status === 409) {
		const err = await res.json().catch(() => ({ title: 'Guest already checked in' }));
		throw new ConflictError(err.title || 'Guest already checked in');
	}
	if (!res.ok) throw new Error(`Failed to check in guest: ${res.status}`);
}

export async function checkOutGuest(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/checkout`, {
		method: 'POST',
	});
	if (!res.ok) throw new Error(`Failed to check out guest: ${res.status}`);
}

export async function assignSeat(weddingId: string, guestId: string, tableId: string, seatNum: number): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, {
		method: 'POST',
		body: JSON.stringify({ tableId, seatNum }),
	});
	if (!res.ok) throw new Error(`Failed to assign seat: ${res.status}`);
}

export async function unassignSeat(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${guestId}/seat`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error(`Failed to unassign seat: ${res.status}`);
}

export async function searchGuests(weddingId: string, query: string): Promise<GuestResponse[]> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error(`Failed to search guests: ${res.status}`);
	return res.json();
}

export async function bulkImportGuests(guests: GuestImportData[]): Promise<{ imported: number }> {
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
