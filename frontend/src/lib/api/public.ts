import type { Guest, BanquetTable } from '$lib/types';
import { weddingId } from '$lib/stores/weddingId';
import { get } from 'svelte/store';

interface PublicGuest {
	id: string;
	name: string;
	rsvp?: string;
	tableId: string | null;
	seatNum: number | null;
	pax: number;
	isVip: boolean;
	pinyin?: string;
}

export function mapGuest(raw: PublicGuest): Guest {
	return {
		id: raw.id,
		name: raw.name ?? '',
		pinyin: raw.pinyin,
		rsvp: (raw.rsvp as Guest['rsvp']) ?? 'no_response',
		pax: raw.pax ?? 1,
		tableId: raw.tableId ?? null,
		seatNumber: raw.seatNum ?? null,
		checkedIn: false, // attendance is staff-only; never expose it to the kiosk
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

export async function publicListGuests(opts: { signal?: AbortSignal } = {}): Promise<Guest[]> {
	const wid = get(weddingId);
	const all: Guest[] = [];
	let cursor: string | undefined;
	do {
		const res = await fetch(`/api/public/weddings/${wid}/guests?limit=1000${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, { signal: opts.signal });
		if (!res.ok) throw new Error('Failed to list guests');
		const data = await res.json();
		all.push(...(data.guests ?? []).map(mapGuest));
		cursor = data.nextCursor ?? undefined;
	} while (cursor);
	return all;
}

export async function publicListTables(): Promise<BanquetTable[]> {
	const wid = get(weddingId);
	const res = await fetch(`/api/public/weddings/${wid}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}
