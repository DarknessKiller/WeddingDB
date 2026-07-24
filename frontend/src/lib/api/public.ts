import type { Guest, BanquetTable } from '$lib/types';

const DEFAULT_WEDDING_ID = 'MQ';

interface PublicGuest {
	id: number;
	name: string;
	phone: string;
	tableId: number | null;
	seatNum: number | null;
	pax: number;
	isVip: boolean;
	checkedInAt: string | null;
}

function mapGuest(raw: PublicGuest): Guest {
	return {
		id: String(raw.id),
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
	const res = await fetch(`/api/public/weddings/${DEFAULT_WEDDING_ID}/guests/search?q=${encodeURIComponent(query)}`);
	if (!res.ok) throw new Error('Search failed');
	const data: PublicGuest[] = await res.json();
	return data.map(mapGuest);
}

export async function publicListGuests(): Promise<Guest[]> {
	const res = await fetch(`/api/public/weddings/${DEFAULT_WEDDING_ID}/guests`);
	if (!res.ok) throw new Error('Failed to list guests');
	const data: PublicGuest[] = await res.json();
	return data.map(mapGuest);
}

export async function publicListTables(): Promise<BanquetTable[]> {
	const res = await fetch(`/api/public/weddings/${DEFAULT_WEDDING_ID}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}
