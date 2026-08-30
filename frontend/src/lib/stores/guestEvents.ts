import { writable, get } from 'svelte/store';
import { selectedGuest } from '$lib/stores';
import { connectSSE, disconnectSSE, type GuestEvent } from '$lib/api/sse';
import { weddingId } from '$lib/stores/weddingId';
import type { Guest, RSVPStatus } from '$lib/types';
import type { GuestResponse } from '$lib/api/guests';

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
 * Seed the stores with the initial guest list (called after first fetch).
 */
export function seedGuests(guests: Guest[]) {
	guestList.set(guests);
	const map = new Map<string, Guest>();
	for (const g of guests) {
		map.set(g.id, g);
	}
	guestMap.set(map);
	recalculateOccupancy();
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

	const guest = eventGuestToGuest(guestData);

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);

		if (type === 'delete') {
			return list.filter((g) => g.id !== guest.id);
		}

		if (idx >= 0) {
			const newList = [...list];
			guest.createdAt = list[idx].createdAt;
			newList[idx] = guest;
			return newList;
		}
		return [...list, guest];
	});

	guestMap.update((map) => {
		const newMap = new Map(map);
		if (type === 'delete') {
			newMap.delete(guest.id);
		} else {
			newMap.set(guest.id, guest);
		}
		return newMap;
	});

	if (type === 'seat_assign' || type === 'checkin' || type === 'checkout' || type === 'delete') {
		recalculateOccupancy();
	}

	// If the drawer is showing this guest, update it live.
	const current = get(selectedGuest);
	if (current?.id === guest.id) {
		selectedGuest.set(guest);
	}
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
	const guest: Guest = {
		id: resp.id,
		name: resp.name,
		phone: resp.phone,
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
		createdAt: new Date(resp.createdAt)
	};

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);
		if (idx < 0) return [...list, guest];
		const newList = [...list];
		guest.createdAt = list[idx].createdAt;
		newList[idx] = guest;
		return newList;
	});
	guestMap.update((map) => new Map(map).set(guest.id, guest));
	recalculateOccupancy();

	const current = get(selectedGuest);
	if (current?.id === guest.id) selectedGuest.set(guest);
}

function eventGuestToGuest(d: NonNullable<GuestEvent['guest']>): Guest {
	return {
		id: d.id,
		name: d.name,
		phone: d.phone,
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
		createdAt: new Date()
	};
}
