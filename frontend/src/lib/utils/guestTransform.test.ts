import { describe, it, expect } from 'vitest';
import { toGuest, toPublicGuest, reviveGuest, applyGuestPatch } from './guestTransform';
import type { GuestResponse } from '#lib/api/guests.ts';

const row: GuestResponse = {
	id: 'g1', weddingId: 'w1', name: 'Alice', pinyin: 'alice', email: 'a@x.io', pax: 2,
	rsvp: 'confirmed', isVip: true, notes: 'n', dietary: ['vegan'], tableId: 't1', seatNum: 3,
	checkedInAt: '2026-01-02T00:00:00Z', angbaoAmt: 88, giftItem: 'vase',
	createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-03T00:00:00Z'
};

describe('guestTransform', () => {
	it('maps API names to the Guest shape', () => {
		const g = toGuest(row);
		expect(g).toMatchObject({ seatNumber: 3, dietaryRequirements: ['vegan'], angbaoAmount: 88, giftItem: 'vase', checkedIn: true, pinyin: 'alice' });
		expect(g.updatedAt?.toISOString()).toBe('2026-01-03T00:00:00.000Z');
	});

	it('maps a public row without carrying staff-only fields', () => {
		const g = toPublicGuest({ id: 'g1', name: 'Alice', tableId: 't1', seatNum: 3, pax: 2, isVip: true, pinyin: 'alice' });
		expect(g).toMatchObject({ id: 'g1', name: 'Alice', tableId: 't1', seatNumber: 3, pax: 2, isVip: true, pinyin: 'alice' });
		expect(g).toMatchObject({ rsvp: 'no_response', checkedIn: false, notes: '', dietaryRequirements: [] });
		expect(g.checkedInAt).toBeUndefined();
		expect(g.email).toBeUndefined();
		expect(g.angbaoAmount).toBeUndefined();
		expect(g.giftItem).toBeUndefined();
	});

	it('revives a cached Guest without losing seat, dietary or angbao', () => {
		const cached = JSON.parse(JSON.stringify([toGuest(row)]));
		const g = reviveGuest(cached[0]);
		expect(g).toMatchObject({ seatNumber: 3, dietaryRequirements: ['vegan'], angbaoAmount: 88, tableId: 't1' });
		expect(g.createdAt).toBeInstanceOf(Date);
		expect(g.checkedInAt).toBeInstanceOf(Date);
	});

	it('applies a patch: undefined keeps, null clears', () => {
		const g = applyGuestPatch(toGuest(row), { seatNum: 5, angbaoAmt: null, tableId: undefined });
		expect(g).toMatchObject({ seatNumber: 5, angbaoAmount: undefined, tableId: 't1', giftItem: 'vase' });
		expect(applyGuestPatch(g, { tableId: null, seatNum: null })).toMatchObject({ tableId: null, seatNumber: null });
	});
});
