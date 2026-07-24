import { apiFetch } from './client';
import { encodeId } from '$lib/utils/encode';

export interface GuestResponse {
	id: string;
	weddingId: number;
	name: string;
	phone: string;
	email: string;
	pax: number;
	rsvp: string;
	isVip: boolean;
	notes: string;
	dietary: string[];
	tableId: number | null;
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
	tableId?: number | null;
	seatNum?: number | null;
	angbaoAmt?: number | null;
	giftItem?: string | null;
}

// ponytail: backend returns raw uint id, frontend needs string
function mapGuest(raw: any): GuestResponse {
	return {
		...raw,
		id: String(raw.id),
	};
}

export async function listGuests(weddingId: string): Promise<{ guests: GuestResponse[]; total: number }> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests`);
	if (!res.ok) throw new Error(`Failed to list guests: ${res.status}`);
	const data = await res.json();
	return { guests: data.guests.map(mapGuest), total: data.total };
}

export async function getGuest(weddingId: string, guestId: string): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}`);
	if (!res.ok) throw new Error(`Failed to get guest: ${res.status}`);
	return mapGuest(await res.json());
}

export async function createGuest(weddingId: string, data: GuestCreateData): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests`, {
		method: 'POST',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error(`Failed to create guest: ${res.status}`);
	return mapGuest(await res.json());
}

export async function updateGuest(weddingId: string, guestId: string, data: GuestCreateData): Promise<GuestResponse> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}`, {
		method: 'PUT',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error(`Failed to update guest: ${res.status}`);
	return mapGuest(await res.json());
}

export async function deleteGuest(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error(`Failed to delete guest: ${res.status}`);
}

export async function checkInGuest(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}/checkin`, {
		method: 'POST',
	});
	if (!res.ok) throw new Error(`Failed to check in guest: ${res.status}`);
}

export async function checkOutGuest(weddingId: string, guestId: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}/checkout`, {
		method: 'POST',
	});
	if (!res.ok) throw new Error(`Failed to check out guest: ${res.status}`);
}

export async function assignSeat(weddingId: string, guestId: string, tableId: string, seatNum: number): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/${encodeId(guestId)}/seat`, {
		method: 'POST',
		body: JSON.stringify({ tableId: encodeId(tableId), seatNum }),
	});
	if (!res.ok) throw new Error(`Failed to assign seat: ${res.status}`);
}

export async function searchGuests(weddingId: string, query: string): Promise<GuestResponse[]> {
	const res = await apiFetch(`/api/weddings/${weddingId}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error(`Failed to search guests: ${res.status}`);
	const data = await res.json();
	return data.map(mapGuest);
}
