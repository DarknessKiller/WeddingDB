import { writable, get } from 'svelte/store';
import { selectedGuest } from '$lib/stores';
import { connectSSE, disconnectSSE, type GuestEvent } from '$lib/api/sse';
import { weddingId } from '$lib/stores/weddingId';
import type { Guest, RSVPStatus } from '$lib/types';

// All guests for the current wedding, kept in sync via SSE.
export const guestList = writable<Guest[]>([]);

// Quick lookup by guest ID.
export const guestMap = writable<Map<string, Guest>>(new Map());

// Table occupancy: tableId -> total pax seated.
export const tableOccupancy = writable<Map<string, number>>(new Map());

// Whether the SSE connection is active.
export const sseConnected = writable(false);

let initialized = false;

/**
 * Initialize the SSE connection and start handling events.
 * Call once from the wedding layout. Returns a cleanup function.
 */
export function initializeSSE(): (() => void) | undefined {
	if (initialized) return undefined;
	initialized = true;

	const wid = get(weddingId);
	if (!wid) {
		initialized = false;
		return undefined;
	}

	const client = connectSSE(wid);
	sseConnected.set(true);

	const unsubscribe = client.onEvent((event: GuestEvent) => {
		handleGuestEvent(event);
	});

	return () => {
		unsubscribe();
		disconnectSSE();
		sseConnected.set(false);
		initialized = false;
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
