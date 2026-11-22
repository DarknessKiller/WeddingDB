import { writable, get } from 'svelte/store';
import { selectedGuest } from '#lib/stores/index.ts';
import { connectSSE, disconnectSSE, type GuestEvent } from '#lib/api/sse.ts';
import { weddingId } from '#lib/stores/weddingId.ts';
import type { Guest } from '#lib/types/index.ts';
import { reviveGuest, toGuest } from '#lib/utils/guestTransform.ts';
import type { GuestResponse } from '#lib/api/guests.ts';

// All guests for the current wedding, kept in sync via SSE.
export const guestList = writable<Guest[]>([]);

// Quick lookup by guest ID.
export const guestMap = writable<Map<string, Guest>>(new Map());

// Table occupancy: tableId -> total pax seated.
export const tableOccupancy = writable<Map<string, number>>(new Map());

// Whether the SSE connection is active.
export const sseConnected = writable(false);

const CACHE_PREFIX = 'offline_cache_';

let initialized = false;
let syncing = false;
let queuedEvents: GuestEvent[] = [];
let resync: (() => Promise<Guest[]>) | undefined;

/**
 * Initialize the SSE connection and start handling events.
 * Call once from the wedding layout. Returns a cleanup function.
 */
export function initializeSSE(onResync?: () => Promise<Guest[]>): (() => void) | undefined {
	if (initialized) return undefined;
	initialized = true;
	syncing = true;
	queuedEvents = [];
	resync = onResync;

	const wid = get(weddingId);
	if (!wid) {
		initialized = false;
		return undefined;
	}

	const client = connectSSE(wid);
	const unsubscribeStatus = client.onStatus((status, reconnect) => {
		sseConnected.set(status === 'connected');
		if (status === 'connected' && reconnect && resync) {
			syncing = true;
			resync().then((list) => seedGuests(list, wid)).catch(() => { syncing = false; });
		}
	});
	const unsubscribe = client.onEvent((event: GuestEvent) => {
		if (syncing) queuedEvents.push(event);
		else handleGuestEvent(event);
	});

	return () => {
		unsubscribe();
		unsubscribeStatus();
		disconnectSSE();
		sseConnected.set(false);
		initialized = false;
		// Leaving a wedding must not leave its guests on screen for the next one.
		clearGuestStores();
		resync = undefined;
	};
}

/**
 * Run a full fetch+seed inside the syncing window: SSE events that arrive
 * mid-fetch are queued and replayed after the seed lands instead of being
 * clobbered by it. Use for manual refreshes outside of initializeSSE
 * (e.g. the wedding layout's online/visibility sync).
 */
export async function refreshGuests(fetchFn: () => Promise<Guest[]>, wid = get(weddingId)): Promise<Guest[]> {
	syncing = true;
	try {
		const guests = await fetchFn();
		seedGuests(guests, wid);
		return guests;
	} catch (e) {
		// Abandon the window; replay anything that queued up meanwhile.
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
		throw e;
	}
}

/**
 * Seed the stores with the initial guest list (called after first fetch).
 * Rows fetched for another wedding are dropped, and the cache is written under
 * the wedding the fetch was for, not the one selected when it finished.
 */
export function seedGuests(guests: Guest[], wid = get(weddingId)) {
	if (wid !== get(weddingId)) return;
	guestList.set(guests);
	const map = new Map<string, Guest>();
	for (const g of guests) {
		map.set(g.id, g);
	}
	guestMap.set(map);
	recalculateOccupancy();
	// cache for offline reads
	try {
		if (wid) localStorage.setItem(CACHE_PREFIX + wid, JSON.stringify(guests));
	} catch {}
	if (syncing) {
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
	}
}

function cacheKey(wid: string) { return CACHE_PREFIX + wid; }

/** The last seeded guest list for `wid`, or null. Rows are app-shaped `Guest`s: only dates need reviving. */
export function readCachedGuests(wid: string): Guest[] | null {
	try {
		const cached = JSON.parse(localStorage.getItem(cacheKey(wid)) || 'null');
		return cached ? (cached as Guest[]).map(reviveGuest) : null;
	} catch {
		return null;
	}
}

/** Empty the guest stores without touching the offline cache. Used when leaving a wedding. */
export function clearGuestStores() {
	syncing = false;
	queuedEvents = [];
	guestList.set([]);
	guestMap.set(new Map());
	tableOccupancy.set(new Map());
	selectedGuest.set(null);
}

/**
 * Drop every cached guest list (they hold emails, notes and angbao amounts).
 * Queued edits are kept: they belong to the offline queue, not the cache.
 */
export function clearGuestCaches() {
	try {
		// Collect first: removing while walking the keys would skip entries.
		const keys: string[] = [];
		for (let i = 0; i < localStorage.length; i++) {
			const k = localStorage.key(i);
			if (k?.startsWith(CACHE_PREFIX)) keys.push(k);
		}
		keys.forEach((k) => localStorage.removeItem(k));
	} catch {}
}

function handleGuestEvent(event: GuestEvent) {
	const { type, guest: guestData } = event;
	if (!guestData) return;
	const incoming = toGuest(guestData);
	// The server's per-column rule already picked the winner; an event older
	// than what this device shows is a late or replayed one and must not undo it.
	const current = get(guestMap).get(incoming.id);
	if (current?.updatedAt && incoming.updatedAt && incoming.updatedAt < current.updatedAt) return;
	if (type === 'delete') return removeGuest(incoming.id);
	upsertGuest(incoming);
}

function recalculateOccupancy() {
	const occ = new Map<string, number>();
	for (const g of get(guestList)) {
		if (g.tableId) occ.set(g.tableId, (occ.get(g.tableId) || 0) + g.pax);
	}
	tableOccupancy.set(occ);
}

/** Move one guest's pax between tables in place, so a live update costs O(1). */
function adjustOccupancy(previous: Guest | undefined, next: Guest | undefined) {
	if (previous?.tableId === next?.tableId && previous?.pax === next?.pax) return;
	tableOccupancy.update((occ) => {
		if (previous?.tableId) {
			const left = (occ.get(previous.tableId) ?? 0) - previous.pax;
			if (left > 0) occ.set(previous.tableId, left);
			else occ.delete(previous.tableId);
		}
		if (next?.tableId) occ.set(next.tableId, (occ.get(next.tableId) ?? 0) + next.pax);
		return occ;
	});
}

/**
 * Cheap belt-and-brace: after a mutation API call succeeds, merge the server
 * response into the stores. SSE normally delivers the event first, so this is
 * usually a same-state replace; it only heals gaps when an event was dropped
 * (SSE hiccup, reconnect race) instead of waiting for a manual refresh.
 */
export function applyGuestResponse(resp: GuestResponse) {
	upsertGuest(toGuest(resp));
}

/** Inserts or replaces a guest in every store. Keeps createdAt, and pinyin when the new row lacks it. */
export function upsertGuest(guest: Guest) {
	const previous = get(guestMap).get(guest.id);
	if (previous) {
		guest.createdAt = previous.createdAt;
		guest.pinyin ??= previous.pinyin;
	}
	// Mutate the arrays and maps in place: a bulk import delivers one event per
	// guest, and a copy per event makes the whole import quadratic.
	guestList.update((list) => {
		const idx = previous ? list.indexOf(previous) : -1;
		if (idx < 0) list.push(guest);
		else list[idx] = guest;
		return list;
	});
	guestMap.update((map) => {
		map.set(guest.id, guest);
		return map;
	});
	adjustOccupancy(previous, guest);

	// If the drawer is showing this guest, update it live.
	const current = get(selectedGuest);
	if (current?.id === guest.id) selectedGuest.set(guest);
}

export function removeGuest(id: string) {
	const previous = get(guestMap).get(id);
	if (!previous) return;
	guestList.update((list) => {
		const idx = list.indexOf(previous);
		if (idx >= 0) list.splice(idx, 1);
		return list;
	});
	guestMap.update((map) => {
		map.delete(id);
		return map;
	});
	adjustOccupancy(previous, undefined);
}
