import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Guest } from '#lib/types/index.ts';

vi.mock('#lib/stores/index.ts', () => ({
	selectedGuest: { set: vi.fn(), update: vi.fn(), subscribe: vi.fn(() => vi.fn()) },
	getAuth: () => ({ accessToken: 'test-token' })
}));

vi.mock('#lib/stores/weddingId.ts', () => ({
	// subscribe must invoke the callback so svelte's get() returns 'w1'
	weddingId: { get: () => 'w1', subscribe: vi.fn((run: (v: string) => void) => { run('w1'); return () => {}; }), set: vi.fn(), update: vi.fn() }
}));

// Captures the SSE event handler so tests can drive handleGuestEvent.
const sseState = vi.hoisted(() => ({ eventHandler: undefined as ((e: unknown) => void) | undefined }));

vi.mock('#lib/api/sse.ts', () => ({
	connectSSE: vi.fn(() => ({
		onEvent: vi.fn((cb: (e: unknown) => void) => { sseState.eventHandler = cb; return () => {}; }),
		onStatus: vi.fn(() => () => {}),
		disconnect: vi.fn()
	})),
	disconnectSSE: vi.fn()
}));

function makeGuest(overrides: Partial<Guest> = {}): Guest {
	return {
		id: overrides.id ?? 'g1',
		name: overrides.name ?? 'Alice',
		email: overrides.email ?? '',
		rsvp: overrides.rsvp ?? 'confirmed',
		pax: overrides.pax ?? 2,
		tableId: 'tableId' in overrides ? overrides.tableId! : 't1',
		seatNumber: overrides.seatNumber ?? 1,
		checkedIn: overrides.checkedIn ?? false,
		checkedInAt: overrides.checkedInAt,
		notes: overrides.notes ?? '',
		dietaryRequirements: overrides.dietaryRequirements ?? [],
		isVip: overrides.isVip ?? false,
		angbaoAmount: overrides.angbaoAmount,
		giftItem: overrides.giftItem,
		createdAt: overrides.createdAt ?? new Date(),
		updatedAt: overrides.updatedAt
	};
}

function makeEvent(overrides: Record<string, unknown> = {}) {
	return {
		type: 'update',
		guestId: 'g1',
		weddingId: 'w1',
		timestamp: Date.now(),
		guest: {
			id: 'g1', name: 'Alice', email: '', pax: 2, rsvp: 'confirmed',
			isVip: false, notes: '', dietary: [], tableId: 't1', seatNum: 1,
			checkedInAt: null, angbaoAmt: null, giftItem: null,
		},
		...overrides,
	};
}

/** Import the module fresh and reset all stores to empty. */
async function freshModule() {
	vi.resetModules();
	const mod = await import('./guestEvents');
	mod.guestList.set([]);
	mod.guestMap.set(new Map());
	mod.tableOccupancy.set(new Map());
	return mod;
}

describe('guestEvents store', () => {
	it('seedGuests populates guestList and guestMap', async () => {
		const { seedGuests, guestList, guestMap } = await freshModule();
		const guests = [
			makeGuest({ id: 'g1', name: 'Alice' }),
			makeGuest({ id: 'g2', name: 'Bob', tableId: 't2' })
		];

		seedGuests(guests);

		const list = get(guestList);
		expect(list).toHaveLength(2);
		expect(list[0].name).toBe('Alice');

		const map = get(guestMap);
		expect(map.size).toBe(2);
		expect(map.get('g1')?.name).toBe('Alice');
		expect(map.get('g2')?.name).toBe('Bob');
	});

	it('seedGuests calculates occupancy', async () => {
		const { seedGuests, tableOccupancy } = await freshModule();
		const guests = [
			makeGuest({ id: 'g1', tableId: 't1', pax: 3 }),
			makeGuest({ id: 'g2', tableId: 't1', pax: 2 }),
			makeGuest({ id: 'g3', tableId: 't2', pax: 4 })
		];

		seedGuests(guests);

		const occ = get(tableOccupancy);
		expect(occ.get('t1')).toBe(5);
		expect(occ.get('t2')).toBe(4);
	});

	it('seedGuests handles guests without tables', async () => {
		const { seedGuests, guestList, tableOccupancy } = await freshModule();
		expect(get(guestList)).toHaveLength(0);

		const guests = [
			makeGuest({ id: 'g1', tableId: null }),
			makeGuest({ id: 'g2', tableId: 't1', pax: 2 })
		];

		seedGuests(guests);

		const list = get(guestList);
		expect(list).toHaveLength(2);
		expect(list.find(g => g.id === 'g1')?.tableId).toBeNull();

		const occ = get(tableOccupancy);
		expect(occ.size).toBe(1);
		expect(occ.get('t1')).toBe(2);
	});

	it('seedGuests replaces previous data', async () => {
		const { seedGuests, guestList } = await freshModule();

		seedGuests([makeGuest({ id: 'g1', name: 'First' })]);
		expect(get(guestList)).toHaveLength(1);

		seedGuests([makeGuest({ id: 'g2', name: 'Second' }), makeGuest({ id: 'g3', name: 'Third' })]);
		const list = get(guestList);
		expect(list).toHaveLength(2);
		expect(list[0].name).toBe('Second');
	});

	it('update events recalculate occupancy (unassign clears table)', async () => {
		const { initializeSSE, seedGuests, tableOccupancy, guestList } = await freshModule();
		initializeSSE(() => Promise.resolve([]));
		seedGuests([makeGuest({ id: 'g1', tableId: 't1', pax: 3 })]);
		expect(get(tableOccupancy).get('t1')).toBe(3);

		sseState.eventHandler!(makeEvent({ guest: { id: 'g1', name: 'Alice', email: '', pax: 2, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: null, angbaoAmt: null, giftItem: null } }));

		expect(get(guestList)[0].tableId).toBeNull();
		expect(get(tableOccupancy).has('t1')).toBe(false);
	});

	it('create events recalculate occupancy', async () => {
		const { initializeSSE, seedGuests, tableOccupancy } = await freshModule();
		initializeSSE(() => Promise.resolve([]));
		seedGuests([]);

		sseState.eventHandler!(makeEvent({ type: 'create', guest: { id: 'g9', name: 'Zoe', email: '', pax: 4, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: 't2', seatNum: 1, checkedInAt: null, angbaoAmt: null, giftItem: null } }));

		expect(get(tableOccupancy).get('t2')).toBe(4);
	});
});

describe('stale wedding data', () => {
	function stubStorage() {
		const store = new Map<string, string>();
		const ls = {
			get length() { return store.size; },
			key: (i: number) => [...store.keys()][i] ?? null,
			getItem: (k: string) => store.get(k) ?? null,
			setItem: (k: string, v: string) => void store.set(k, v),
			removeItem: (k: string) => void store.delete(k)
		};
		vi.stubGlobal('localStorage', ls);
		return store;
	}

	it('drops a list fetched for a previous wedding', async () => {
		const store = stubStorage();
		try {
			const { seedGuests, guestList } = await freshModule();
			// Current wedding is 'w1'; these rows belong to the one opened before.
			seedGuests([makeGuest({ id: 'old' })], 'w0');

			expect(get(guestList)).toHaveLength(0);
			expect(store.has('offline_cache_w0')).toBe(false);
		} finally { vi.unstubAllGlobals(); }
	});

	it('a dropped result closes the syncing window so live events still apply', async () => {
		const { initializeSSE, seedGuests, guestList } = await freshModule();
		initializeSSE(() => Promise.resolve([]));
		// Late rows for the wedding we already left.
		seedGuests([makeGuest({ id: 'late' })], 'w0');

		// A queued-and-never-replayed sync window would swallow this event.
		sseState.eventHandler!(makeEvent({ type: 'create', guest: { id: 'g9', name: 'Zoe', email: '', pax: 1, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: null, angbaoAmt: null, giftItem: null } }));

		expect(get(guestList).map(g => g.id)).toEqual(['g9']);
	});

	it('clearGuestStores empties the stores and the selected guest', async () => {
		const { seedGuests, clearGuestStores, guestList, guestMap, tableOccupancy } = await freshModule();
		seedGuests([makeGuest({ id: 'g1', tableId: 't1', pax: 2 })]);

		clearGuestStores();

		expect(get(guestList)).toHaveLength(0);
		expect(get(guestMap).size).toBe(0);
		expect(get(tableOccupancy).size).toBe(0);
	});

	it('clearGuestCaches drops cached guest lists but keeps queued offline edits', async () => {
		const store = stubStorage();
		try {
			const { seedGuests, clearGuestCaches } = await freshModule();
			seedGuests([makeGuest({ email: 'a@b.c' })], 'w1');
			store.set('offline_queue_w1', '[{"mutationId":"m1"}]');

			clearGuestCaches();

			expect(store.has('offline_cache_w1')).toBe(false);
			expect(store.has('offline_queue_w1')).toBe(true);
		} finally { vi.unstubAllGlobals(); }
	});

	it('ignores an event older than the row on screen', async () => {
		const { initializeSSE, seedGuests, guestList } = await freshModule();
		initializeSSE(() => Promise.resolve([]));
		seedGuests([makeGuest({ id: 'g1', name: 'Newer', updatedAt: new Date('2026-01-01T10:00:00Z') })]);

		// A late/replayed event from before the write that is already shown.
		sseState.eventHandler!(makeEvent({ guest: { id: 'g1', name: 'Older', email: '', pax: 2, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: 't1', seatNum: 1, checkedInAt: null, angbaoAmt: null, giftItem: null, updatedAt: '2026-01-01T09:00:00Z' } }));
		expect(get(guestList)[0].name).toBe('Newer');

		sseState.eventHandler!(makeEvent({ guest: { id: 'g1', name: 'Newest', email: '', pax: 2, rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: 't1', seatNum: 1, checkedInAt: null, angbaoAmt: null, giftItem: null, updatedAt: '2026-01-01T11:00:00Z' } }));
		expect(get(guestList)[0].name).toBe('Newest');
	});
});

describe('offline guest cache', () => {
	it('reads back what seedGuests cached without losing seat, dietary or angbao', async () => {
		const store = new Map<string, string>();
		vi.stubGlobal('localStorage', { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) });
		try {
			const { seedGuests, readCachedGuests } = await freshModule();
			seedGuests([makeGuest({ tableId: 't1', seatNumber: 3, dietaryRequirements: ['vegan'], angbaoAmount: 88, checkedIn: true, checkedInAt: new Date('2026-01-01T10:00:00Z') })]);

			const [cached] = readCachedGuests('w1') ?? [];
			expect(cached).toMatchObject({ tableId: 't1', seatNumber: 3, dietaryRequirements: ['vegan'], angbaoAmount: 88, checkedIn: true });
			expect(cached.checkedInAt).toBeInstanceOf(Date);
			expect(readCachedGuests('other')).toBeNull();
		} finally {
			vi.unstubAllGlobals();
		}
	});
});
