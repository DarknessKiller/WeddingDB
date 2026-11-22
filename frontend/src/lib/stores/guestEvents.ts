import { writable, derived, get } from 'svelte/store';
import { selectedGuest } from '#lib/stores/index.ts';
import { connectSSE, disconnectSSE, type GuestEvent } from '#lib/api/sse.ts';
import { weddingId } from '#lib/stores/weddingId.ts';
import type { Guest, RSVPStatus } from '#lib/types/index.ts';
import type { GuestResponse } from '#lib/api/guests.ts';

// All guests for the current wedding, kept in sync via SSE.
export const guestList = writable<Guest[]>([]);

// Whether the initial snapshot has landed and whether the last refresh failed.
// The guests page renders from this instead of assuming an empty list is
// emptier than the server's.
export const guestSync = writable<{ loading: boolean; error: string | null }>({ loading: true, error: null });

// Table occupancy: tableId -> total pax seated. Derived so every guestList
// write (seed, SSE, optimistic offline patch) keeps it current.
export const tableOccupancy = derived(guestList, (guests) => {
	const occ = new Map<string, number>();
	for (const g of guests) {
		if (g.tableId) occ.set(g.tableId, (occ.get(g.tableId) || 0) + g.pax);
	}
	return occ;
});



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
		if (status === 'connected' && reconnect && resync) {
			// Show the failure instead of keeping a silently stale list.
			refreshGuests(resync).catch((e: unknown) => {
				guestSync.set({ loading: false, error: e instanceof Error ? e.message : 'Failed to refresh guests' });
			});
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
		initialized = false;
		syncing = false;
		queuedEvents = [];
		resync = undefined;
	};
}

let refreshChain: Promise<unknown> = Promise.resolve();

/**
 * Run a full fetch+seed inside the syncing window. Refreshes are serialized so
 * two overlapping calls cannot apply their snapshots out of order.
 */
export function refreshGuests(fetchFn: () => Promise<Guest[]>, wid = get(weddingId)): Promise<Guest[]> {
	const run = refreshChain.then(() => runRefresh(fetchFn, wid));
	refreshChain = run.catch(() => {});
	return run;
}

async function runRefresh(fetchFn: () => Promise<Guest[]>, wid: string): Promise<Guest[]> {
	syncing = true;
	guestSync.set({ loading: true, error: null });
	try {
		const guests = await fetchFn();
		// The layout switched weddings while this request was in flight: its rows
		// and the SSE events queued meanwhile belong to the old wedding.
		if (get(weddingId) !== wid) {
			syncing = false;
			queuedEvents = [];
			return guests;
		}
		seedGuests(guests);
		return guests;
	} catch (e) {
		// A stale request must not paint an error over the current wedding.
		if (get(weddingId) === wid) {
			guestSync.set({ loading: false, error: e instanceof Error ? e.message : 'Failed to load guests' });
		}
		// Abandon the window; replay anything that queued up meanwhile.
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
		throw e;
	}
}

/** Retry the last snapshot fetch after a failed load or reconnect resync. */
export async function retryGuestSync(): Promise<void> {
	if (!resync) return;
	await refreshGuests(resync).catch(() => {});
}

/**
 * Seed the stores with the initial guest list (called after first fetch).
 */
export function seedGuests(guests: Guest[]) {
	guestList.set(guests);
	guestSync.set({ loading: false, error: null });
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

	const guest = eventGuestToGuest(guestData, event.timestamp);

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);

		if (type === 'delete') {
			return list.filter((g) => g.id !== guest.id);
		}

		if (idx >= 0) {
			const newList = [...list];
			guest.createdAt = list[idx].createdAt;
			// SSE payloads carry no pinyin; keep the seeded value or local
			// pinyin search stops matching after any edit.
			guest.pinyin = guest.pinyin ?? list[idx].pinyin;
			newList[idx] = guest;
			return newList;
		}
		return [...list, guest];
	});


	// If the drawer is showing this guest, update it live.
	const current = get(selectedGuest);
	if (current?.id === guest.id) {
		selectedGuest.set(guest);
	}
}

/**
 * Cheap belt-and-brace: after a mutation API call succeeds, merge the server
 * response into the stores. SSE normally delivers the event first, so this is
 * usually a same-state replace; it only heals gaps when an event was dropped
 * (SSE hiccup, reconnect race) instead of waiting for a manual refresh.
 */
export function applyGuestResponse(resp: GuestResponse) {
	const guest: Guest = {
		id: resp.id,
		name: resp.name,
		pinyin: resp.pinyin,
		email: resp.email,
		rsvp: resp.rsvp as RSVPStatus,
		pax: resp.pax,
		tableId: resp.tableId,
		seatNumber: resp.seatNum,
		checkedIn: !!resp.checkedInAt,
		checkedInAt: resp.checkedInAt ? new Date(resp.checkedInAt) : undefined,
		notes: resp.notes,
		dietaryRequirements: resp.dietary,
		isVip: resp.isVip,
		angbaoAmount: resp.angbaoAmt ?? undefined,
		giftItem: resp.giftItem ?? undefined,
		createdAt: new Date(resp.createdAt),
		updatedAt: new Date(resp.updatedAt)
	};

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);
		if (idx < 0) return [...list, guest];
		const newList = [...list];
		guest.createdAt = list[idx].createdAt;
		newList[idx] = guest;
		return newList;
	});

	const current = get(selectedGuest);
	if (current?.id === guest.id) selectedGuest.set(guest);
}

function eventGuestToGuest(d: NonNullable<GuestEvent['guest']>, eventTimestamp: number): Guest {
	const updatedAt = eventTimestamp > 0 ? new Date(eventTimestamp) : new Date();
	return {
		id: d.id,
		name: d.name,
		email: d.email,
		rsvp: d.rsvp as RSVPStatus,
		pax: d.pax,
		tableId: d.tableId,
		seatNumber: d.seatNum,
		checkedIn: !!d.checkedInAt,
		checkedInAt: d.checkedInAt ? new Date(d.checkedInAt) : undefined,
		notes: d.notes,
		dietaryRequirements: d.dietary,
		isVip: d.isVip,
		angbaoAmount: d.angbaoAmt ?? undefined,
		giftItem: d.giftItem ?? undefined,
		createdAt: updatedAt,
		updatedAt
	};
}

/** Drop a guest the server no longer has (deleted on another device). */
export function removeGuest(id: string) {
	guestList.update((list) => list.filter((g) => g.id !== id));
	if (get(selectedGuest)?.id === id) selectedGuest.set(null);
}
