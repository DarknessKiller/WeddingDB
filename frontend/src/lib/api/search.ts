import { apiFetch } from './client';
import { encodeId } from '$lib/utils/encode';
import type { Guest, BanquetTable } from '$lib/types';

// ponytail: default weddingId = base64url("1"), swap later if multi-wedding needed
const DEFAULT_WEDDING_ID = 'MQ';

export async function searchGuests(query: string): Promise<Guest[]> {
	const res = await apiFetch(`/api/weddings/${DEFAULT_WEDDING_ID}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error('Search failed');
	const data = await res.json();
	return data.map(mapGuest);
}

export async function getGuest(guestId: string): Promise<Guest | null> {
	const res = await apiFetch(`/api/weddings/${DEFAULT_WEDDING_ID}/guests/${encodeId(guestId)}`);
	if (!res.ok) return null;
	return mapGuest(await res.json());
}

export async function listGuests(): Promise<Guest[]> {
	const res = await apiFetch(`/api/weddings/${DEFAULT_WEDDING_ID}/guests`);
	if (!res.ok) throw new Error('Failed to list guests');
	const data = await res.json();
	return data.guests.map(mapGuest);
}

export async function listTables(): Promise<BanquetTable[]> {
	const res = await apiFetch(`/api/weddings/${DEFAULT_WEDDING_ID}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}

export async function checkInGuest(guestId: string, angbaoAmt?: number, giftItem?: string): Promise<Guest> {
	const body: Record<string, unknown> = {};
	if (angbaoAmt !== undefined) body.angbaoAmt = angbaoAmt;
	if (giftItem) body.giftItem = giftItem;
	const res = await apiFetch(`/api/weddings/${DEFAULT_WEDDING_ID}/guests/${encodeId(guestId)}/checkin`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
	if (!res.ok) throw new Error('Check-in failed');
	return mapGuest(await res.json());
}

function mapGuest(raw: any): Guest {
	return {
		id: String(raw.id),
		name: raw.name ?? '',
		phone: raw.phone ?? '',
		email: raw.email,
		rsvp: raw.rsvp ?? 'no_response',
		pax: raw.pax ?? 1,
		tableId: raw.tableId ?? null,
		seatNumber: raw.seatNum ?? null,
		checkedIn: !!raw.checkedInAt,
		checkedInAt: raw.checkedInAt ? new Date(raw.checkedInAt) : undefined,
		notes: raw.notes ?? '',
		dietaryRequirements: raw.dietary ?? [],
		isVip: raw.isVip ?? false,
		angbaoAmount: raw.angbaoAmt ?? undefined,
		giftItem: raw.giftItem ?? undefined,
		createdAt: raw.createdAt ? new Date(raw.createdAt) : new Date()
	};
}
