import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { Guest } from '$lib/types';

vi.mock('$lib/stores', () => ({
	selectedGuest: { set: vi.fn(), update: vi.fn(), subscribe: vi.fn(() => vi.fn()) },
	getAuth: () => ({ accessToken: 'test-token' })
}));

vi.mock('$lib/stores/weddingId', () => ({
	weddingId: { subscribe: vi.fn(() => vi.fn()), set: vi.fn(), update: vi.fn() }
}));

vi.mock('$lib/api/sse', () => ({
	connectSSE: vi.fn(() => ({
		onEvent: vi.fn(() => vi.fn()),
		disconnect: vi.fn()
	})),
	disconnectSSE: vi.fn()
}));

function makeGuest(overrides: Partial<Guest> = {}): Guest {
	return {
		id: overrides.id ?? 'g1',
		name: overrides.name ?? 'Alice',
		phone: overrides.phone ?? '123',
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
		createdAt: overrides.createdAt ?? new Date()
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
});
