import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { seedGuests, applyGuestResponse, guestList } from './guestEvents';
import type { GuestResponse } from '$lib/api/guests';

vi.mock('$lib/stores', () => ({
	selectedGuest: { get: () => null, set: vi.fn(), update: vi.fn(), subscribe: vi.fn(() => vi.fn()) }
}));
vi.mock('$lib/api/sse', () => ({
	connectSSE: () => ({ onEvent: () => () => {}, onStatus: () => () => {} }),
	disconnectSSE: () => {}
}));
vi.mock('$lib/stores/weddingId', () => ({
	weddingId: { get: () => 'w1', subscribe: vi.fn(() => vi.fn()), set: vi.fn(), update: vi.fn() }
}));

function resp(id: string, name = 'Alice'): GuestResponse {
	return {
		id, weddingId: 'w1', name, phone: '', email: '', pax: 2,
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
