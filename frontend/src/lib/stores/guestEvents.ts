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
			resync().then(seedGuests).catch(() => { syncing = false; });
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
		syncing = false;
		queuedEvents = [];
		resync = undefined;
	};
}

/**
 * Run a full fetch+seed inside the syncing window: SSE events that arrive
 * mid-fetch are queued and replayed after the seed lands instead of being
 * clobbered by it. Use for manual refreshes outside of initializeSSE
 * (e.g. the wedding layout's online/visibility sync).
 */
export async function refreshGuests(fetchFn: () => Promise<Guest[]>): Promise<Guest[]> {
	syncing = true;
	try {
		const guests = await fetchFn();
		seedGuests(guests);
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
 */
function cacheKey(wid: string) { return `offline_cache_${wid}`; }

/** The last seeded guest list for `wid`, or null. Rows are app-shaped `Guest`s: only dates need reviving. */
export function readCachedGuests(wid: string): Guest[] | null {
	try {
		const cached = JSON.parse(localStorage.getItem(cacheKey(wid)) || 'null');
		return cached ? (cached as Guest[]).map(reviveGuest) : null;
	} catch {
		return null;
	}
}

export function seedGuests(guests: Guest[]) {
	guestList.set(guests);
	const map = new Map<string, Guest>();
	for (const g of guests) {
		map.set(g.id, g);
	}
	guestMap.set(map);
	recalculateOccupancy();
	// cache for offline reads
	try {
		const wid = get(weddingId);
		if (wid) localStorage.setItem(cacheKey(wid), JSON.stringify(guests));
	} catch {}
	if (syncing) {
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
	}
}

function handleGuestEvent(event: GuestEvent) {
	const { type, guest: guestData } = event;
	if (!guestData) return;
	if (type === 'delete') return removeGuest(guestData.id);
	const updatedAt = event.timestamp > 0 ? new Date(event.timestamp).toISOString() : undefined;
	upsertGuest(toGuest({ ...guestData, updatedAt }));
}

function recalculateOccupancy() {
	const guests = get(guestList);
	const occ = new Map<string, number>();
	for (const g of guests) {
		if (g.tableId) {
			occ.set(g.tableId, (occ.get(g.tableId) || 0) + g.pax);
		}
	}
	tableOccupancy.set(occ);
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
	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);
		if (idx < 0) return [...list, guest];
		const newList = [...list];
		guest.createdAt = list[idx].createdAt;
		guest.pinyin ??= list[idx].pinyin;
		newList[idx] = guest;
		return newList;
	});
	guestMap.update((map) => new Map(map).set(guest.id, guest));
	recalculateOccupancy();

	// If the drawer is showing this guest, update it live.
	const current = get(selectedGuest);
	if (current?.id === guest.id) selectedGuest.set(guest);
}

export function removeGuest(id: string) {
	guestList.update((list) => list.filter((g) => g.id !== id));
	guestMap.update((map) => {
		const newMap = new Map(map);
		newMap.delete(id);
		return newMap;
	});
	recalculateOccupancy();
}
