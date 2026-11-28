import type { Guest } from '#lib/types/index.ts';
import { weddingId } from '#lib/stores/weddingId.ts';
import { get } from 'svelte/store';
import { withTimeout } from './client';
import { toPublicGuest, type PublicGuestRow } from '#lib/utils/guestTransform.ts';

export async function publicListGuests(opts: { signal?: AbortSignal; wid?: string } = {}): Promise<Guest[]> {
	// Callers on a reused component pass the route param: the store can still
	// hold the previous wedding when the page loads.
	const wid = opts.wid || get(weddingId);
	const all: Guest[] = [];
	let cursor: string | undefined;
	do {
		const res = await fetch(`/api/public/weddings/${wid}/guests?limit=1000${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, {
			// A hung request would block every later kiosk poll; cap it.
			signal: withTimeout(15_000, opts.signal)
		});
		if (!res.ok) throw new Error('Failed to list guests');
		const data = await res.json();
		all.push(...(data.guests ?? []).map((raw: PublicGuestRow) => toPublicGuest(raw)));
		cursor = data.nextCursor ?? undefined;
	} while (cursor);
	return all;
}
