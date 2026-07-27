import { apiFetch } from './client';
import { weddingId } from '$lib/stores/weddingId';
import { get } from 'svelte/store';
import type { Guest, BanquetTable, RSVPStatus } from '$lib/types';

const VALID_RSVP = ['confirmed', 'pending', 'declined', 'no_response'];
function safeRsvp(v: unknown): RSVPStatus {
  return VALID_RSVP.includes(v as string) ? (v as RSVPStatus) : 'no_response';
}

export async function searchGuests(query: string): Promise<Guest[]> {
	const wid = get(weddingId);
	const res = await apiFetch(`/api/weddings/${wid}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error('Search failed');
	const data = await res.json();
	return data.map(mapGuest);
}

export async function getGuest(guestId: string): Promise<Guest | null> {
	const wid = get(weddingId);
	const res = await apiFetch(`/api/weddings/${wid}/guests/${guestId}`);
	if (!res.ok) return null;
	return mapGuest(await res.json());
}

export async function listGuests(): Promise<Guest[]> {
	const wid = get(weddingId);
	const all: Guest[] = [];
	let cursor: string | undefined;
	do {
		const res = await apiFetch(`/api/weddings/${wid}/guests?limit=100${cursor ? `&cursor=${cursor}` : ''}`);
		if (!res.ok) throw new Error('Failed to list guests');
		const data = await res.json();
		all.push(...data.guests.map(mapGuest));
		cursor = data.nextCursor ?? undefined;
	} while (cursor);
	return all;
}

export async function listTables(): Promise<BanquetTable[]> {
	const wid = get(weddingId);
	const res = await apiFetch(`/api/weddings/${wid}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}

export async function checkInGuest(guestId: string, angbaoAmt?: number, giftItem?: string): Promise<Guest> {
	const wid = get(weddingId);
	const body: Record<string, unknown> = {};
	if (angbaoAmt !== undefined) body.angbaoAmt = angbaoAmt;
	if (giftItem) body.giftItem = giftItem;
	const res = await apiFetch(`/api/weddings/${wid}/guests/${guestId}/checkin`, {
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
		rsvp: safeRsvp(raw.rsvp),
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
