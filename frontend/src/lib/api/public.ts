import type { Guest, BanquetTable } from '$lib/types';
import { weddingId } from '$lib/stores/weddingId';
import { get } from 'svelte/store';

interface PublicGuest {
	id: string;
	name: string;
	phone: string;
	tableId: string | null;
	seatNum: number | null;
	pax: number;
	isVip: boolean;
	checkedInAt: string | null;
}

function mapGuest(raw: PublicGuest): Guest {
	return {
		id: raw.id,
		name: raw.name ?? '',
		phone: raw.phone ?? '',
		rsvp: 'no_response',
		pax: raw.pax ?? 1,
		tableId: raw.tableId ?? null,
		seatNumber: raw.seatNum ?? null,
		checkedIn: !!raw.checkedInAt,
		checkedInAt: raw.checkedInAt ? new Date(raw.checkedInAt) : undefined,
		notes: '',
		dietaryRequirements: [],
		isVip: raw.isVip ?? false,
		createdAt: new Date()
	};
}

export async function publicSearchGuests(query: string): Promise<Guest[]> {
	const wid = get(weddingId);
	const res = await fetch(`/api/public/weddings/${wid}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error('Search failed');
	const data: PublicGuest[] = await res.json();
	return data.map(mapGuest);
}

export async function publicListGuests(): Promise<Guest[]> {
	const wid = get(weddingId);
	const res = await fetch(`/api/public/weddings/${wid}/guests`);
	if (!res.ok) throw new Error('Failed to list guests');
	const data: PublicGuest[] = await res.json();
	return data.map(mapGuest);
}

export async function publicListTables(): Promise<BanquetTable[]> {
	const wid = get(weddingId);
	const res = await fetch(`/api/public/weddings/${wid}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}
