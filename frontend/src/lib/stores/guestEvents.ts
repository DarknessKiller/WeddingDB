import { writable, get } from 'svelte/store';
import { selectedGuest } from '#lib/stores/index.ts';
import { connectSSE, disconnectSSE, type GuestEvent } from '#lib/api/sse.ts';
import { weddingId } from '#lib/stores/weddingId.ts';
import type { Guest, RSVPStatus } from '#lib/types/index.ts';
import type { GuestResponse } from '#lib/api/guests.ts';
import { cacheGuests, syncQueue } from '#lib/offline/queue.ts';

// All guests for the current wedding, kept in sync via SSE.
export const guestList = writable<Guest[]>([]);

// Table occupancy: tableId -> total pax seated.
export const tableOccupancy = writable<Map<string, number>>(new Map());

// Whether the current wedding's roster has loaded. 'error' means the first load
// failed and no roster (live or cached) is shown, so pages must not render an
// empty list as "no guests".
export const rosterStatus = writable<'loading' | 'ready' | 'error'>('loading');

let initialized = false;
// True once the stores hold the wedding's full roster (seedGuests), false
// after clearGuests. Only a full roster may overwrite the offline cache.
let seeded = false;
let syncing = false;
let queuedEvents: GuestEvent[] = [];
// Bumped by every refreshGuests call; only the newest may seed or close the
// syncing window, so an older snapshot landing late cannot overwrite events.
let refreshGen = 0;
let resync: (() => Promise<Guest[]>) | undefined;
let cacheTimer: ReturnType<typeof setTimeout> | undefined;

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
	const unsubscribeStatus = client.onStatus((status) => {
		if (status === 'connected' && resync) {
			// Send edits queued while offline first, so the reseed shows them.
			syncQueue(wid).catch(() => {}).then(() => {
				if (resync && get(weddingId) === wid) return refreshGuests(resync);
			}).catch(() => {});
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

/**
 * Run a full fetch+seed inside the syncing window: SSE events that arrive
 * mid-fetch are queued and replayed after the seed lands instead of being
 * clobbered by it. Use for manual refreshes outside of initializeSSE
 * (e.g. the wedding layout's online/visibility sync).
 */
export async function refreshGuests(fetchFn: () => Promise<Guest[]>): Promise<Guest[]> {
	const wid = get(weddingId);
	const gen = ++refreshGen;
	syncing = true;
	try {
		const guests = await fetchFn();
		// A newer refresh started after this one: its snapshot is fresher and it
		// owns the syncing window, so this one must not seed.
		if (gen !== refreshGen) return guests;
		// The wedding changed mid-fetch: this roster belongs to the old one, so it
		// must not be shown or cached under the new id. The new wedding's own load seeds.
		if (get(weddingId) !== wid) return guests;
		seedGuests(guests);
		return guests;
	} catch (e) {
		if (gen !== refreshGen) throw e;
		if (!seeded && get(weddingId) === wid) rosterStatus.set('error');
		// Abandon the window; replay anything that queued up meanwhile.
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
		throw e;
	}
}

/** Re-run the current wedding's roster load, e.g. from a Retry button. */
export async function retryRoster(): Promise<void> {
	if (!resync) return;
	rosterStatus.set('loading');
	await refreshGuests(resync).catch(() => {});
}

/**
 * Seed the stores with the initial guest list (called after first fetch).
 */
export function seedGuests(guests: Guest[]) {
	guestList.set(guests);
	seeded = true;
	rosterStatus.set('ready');
	recalculateOccupancy();
	// cache for offline reads
	try {
		const wid = get(weddingId);
		if (wid) cacheGuests(wid, guests);
	} catch {}
	if (syncing) {
		syncing = false;
		const pending = queuedEvents;
		queuedEvents = [];
		pending.forEach(handleGuestEvent);
	}
}

/**
 * Clear the guest stores without caching, for a wedding/user switch. Seeding an
 * empty list here would overwrite the next wedding's offline cache.
 */
export function clearGuests() {
	seeded = false;
	rosterStatus.set('loading');
	guestList.set([]);
	tableOccupancy.set(new Map());
}

// Events carry the canonical UUID; the app holds the base64url form the REST API uses.
function sameWedding(eventWid: string, wid: string): boolean {
	if (eventWid === wid) return true;
	const hex = eventWid.replace(/-/g, '');
	if (!/^[0-9a-f]{32}$/i.test(hex)) return false;
	const bytes = String.fromCharCode(...hex.match(/../g)!.map((h) => parseInt(h, 16)));
	return btoa(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '') === wid;
}

function handleGuestEvent(event: GuestEvent) {
	const { type, guest: guestData } = event;
	if (!guestData) return;
	// A stream left over from another wedding must not touch this roster.
	if (!sameWedding(event.weddingId, get(weddingId))) return;

	const guest = toGuest(guestData, event.timestamp > 0 ? new Date(event.timestamp) : new Date());

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);

		if (type === 'delete') {
			return list.filter((g) => g.id !== guest.id);
		}

		if (idx >= 0) {
			// Out-of-order delivery after a reconnect must not roll a guest
			// back to an older row.
			if (event.timestamp > 0 && (list[idx].updatedAt?.getTime() ?? 0) > event.timestamp) return list;
			const newList = [...list];
			guest.createdAt = list[idx].createdAt;
			// Older servers send no pinyin in events; keep the REST one so search still works.
			guest.pinyin ??= list[idx].pinyin;
			newList[idx] = guest;
			return newList;
		}
		return [...list, guest];
	});

	// 'create'/'update' events also carry table/seat/pax changes, so occupancy
	// must be recalculated for them too — otherwise $tableOccupancy goes stale
	// after REST updates or unassigns (the server publishes those as 'update').
	if (type === 'create' || type === 'update' || type === 'seat_assign' || type === 'checkin' || type === 'checkout' || type === 'delete') {
		recalculateOccupancy();
	}

	// If the drawer is showing this guest, update it live.
	const current = get(selectedGuest);
	if (current?.id === guest.id) {
		selectedGuest.set(guest);
	}
	cacheGuestList();
}

/**
 * Re-cache the live roster after a local or SSE change, so a reload while
 * offline shows it. Skipped until a full roster is seeded: caching a partial
 * list would overwrite the wedding's offline copy.
 */
export function cacheGuestList() {
	if (!seeded) return;
	// An import fires one event per guest; writing the whole roster for each
	// froze phones. One trailing write covers the burst.
	clearTimeout(cacheTimer);
	cacheTimer = setTimeout(() => {
		try {
			const wid = get(weddingId);
			if (wid) cacheGuests(wid, get(guestList));
		} catch {}
	}, 500);
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
	const guest = toGuest(resp);

	guestList.update((list) => {
		const idx = list.findIndex((g) => g.id === guest.id);
		if (idx < 0) return [...list, guest];
		const newList = [...list];
		guest.createdAt = list[idx].createdAt;
		newList[idx] = guest;
		return newList;
	});
	recalculateOccupancy();

	const current = get(selectedGuest);
	if (current?.id === guest.id) selectedGuest.set(guest);
	cacheGuestList();
}

// A guest as the server sends it: REST responses carry timestamps, SSE events do not.
type GuestWire = Omit<GuestResponse, 'weddingId' | 'createdAt' | 'updatedAt'> & { createdAt?: string; updatedAt?: string };

/** The one server-to-Guest mapper for staff data. Go sends an empty list as null. */
export function toGuest(r: GuestWire, fallbackTime = new Date()): Guest {
	const createdAt = r.createdAt ? new Date(r.createdAt) : fallbackTime;
	return {
		id: r.id,
		name: r.name,
		pinyin: r.pinyin,
		email: r.email || undefined,
		rsvp: r.rsvp as RSVPStatus,
		pax: r.pax,
		tableId: r.tableId ?? null,
		seatNumber: r.seatNum ?? null,
		checkedIn: !!r.checkedInAt,
		checkedInAt: r.checkedInAt ? new Date(r.checkedInAt) : undefined,
		notes: r.notes ?? '',
		dietaryRequirements: r.dietary ?? [],
		isVip: r.isVip,
		angbaoAmount: r.angbaoAmt ?? undefined,
		giftItem: r.giftItem ?? undefined,
		createdAt,
		updatedAt: r.updatedAt ? new Date(r.updatedAt) : createdAt
	};
}

/** Restore a roster read back from the offline cache: JSON turned its dates into strings. */
export function reviveCachedGuest(g: Guest): Guest {
	return {
		...g,
		checkedInAt: g.checkedInAt ? new Date(g.checkedInAt) : undefined,
		createdAt: new Date(g.createdAt),
		updatedAt: g.updatedAt ? new Date(g.updatedAt) : undefined
	};
}
