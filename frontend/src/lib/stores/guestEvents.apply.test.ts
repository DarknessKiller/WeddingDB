import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { seedGuests, applyGuestResponse, guestList, toGuest, reviveCachedGuest } from './guestEvents';
import type { GuestResponse } from '#lib/api/guests.ts';

vi.mock('#lib/stores/index.ts', () => ({
	selectedGuest: { get: () => null, set: vi.fn(), update: vi.fn(), subscribe: vi.fn(() => vi.fn()) }
}));
vi.mock('#lib/api/sse.ts', () => ({
	connectSSE: () => ({ onEvent: () => () => {}, onStatus: () => () => {} }),
	disconnectSSE: () => {}
}));
vi.mock('#lib/stores/weddingId.ts', () => ({
	weddingId: { get: () => 'w1', subscribe: vi.fn(() => vi.fn()), set: vi.fn(), update: vi.fn() }
}));

function resp(id: string, name = 'Alice'): GuestResponse {
	return {
		id, weddingId: 'w1', name, email: '', pax: 2,
		rsvp: 'confirmed', isVip: false, notes: '', dietary: [], tableId: null,
		seatNum: null, checkedInAt: null, angbaoAmt: null, giftItem: null,
		createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z'
	};
}

beforeEach(() => {
	seedGuests([]);
});

describe('applyGuestResponse', () => {
	it('appends a guest missing from the list (heals dropped SSE event)', () => {
		applyGuestResponse(resp('g1'));
		expect(get(guestList).map((g) => g.id)).toEqual(['g1']);
		expect(get(guestList)[0].updatedAt?.toISOString()).toBe('2026-01-01T00:00:00.000Z');
	});

	it('replaces in place without duplicating on repeated responses', () => {
		applyGuestResponse(resp('g1', 'Alice'));
		applyGuestResponse(resp('g1', 'Alicia'));
		const list = get(guestList);
		expect(list).toHaveLength(1);
		expect(list[0].name).toBe('Alicia');
	});
});

describe('toGuest', () => {
	it('maps a null dietary list (Go nil slice) to an empty array', () => {
		const g = toGuest({ ...resp('g1'), dietary: null as unknown as string[] });
		expect(g.dietaryRequirements).toEqual([]);
	});

	it('round-trips a cached roster without losing seat or angbao', () => {
		const g = toGuest({ ...resp('g1'), tableId: 't1', seatNum: 3, angbaoAmt: 200, checkedInAt: '2026-01-01T10:00:00Z' });
		const back = reviveCachedGuest(JSON.parse(JSON.stringify(g)));
		expect(back).toEqual(g);
	});
});

