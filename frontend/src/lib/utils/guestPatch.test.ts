import { describe, expect, it } from 'vitest';
import type { Guest } from '#lib/types/index.ts';
import { guestPatch, type GuestFormValues } from './guestPatch';

function guest(overrides: Partial<Guest> = {}): Guest {
	return {
		id: 'g1',
		name: 'Alice',
		email: 'alice@example.com',
		rsvp: 'pending',
		pax: 2,
		tableId: null,
		seatNumber: null,
		checkedIn: false,
		notes: 'vegetarian',
		dietaryRequirements: ['vegetarian'],
		isVip: false,
		angbaoAmount: 100,
		giftItem: 'Tea set',
		createdAt: new Date('2026-01-01'),
		updatedAt: new Date('2026-01-02'),
		...overrides,
	};
}

function form(overrides: Partial<GuestFormValues> = {}): GuestFormValues {
	return {
		name: 'Alice',
		email: 'alice@example.com',
		pax: 2,
		rsvp: 'pending',
		isVip: false,
		notes: 'vegetarian',
		dietary: ['vegetarian'],
		angbaoAmt: '100',
		giftItem: 'Tea set',
		...overrides,
	};
}

describe('guestPatch', () => {
	it('omits unchanged fields', () => {
		expect(guestPatch(guest(), form())).toEqual({});
	});

	it('includes cleared strings as empty strings', () => {
		expect(guestPatch(guest(), form({ email: '', notes: '' }))).toEqual({ email: '', notes: '' });
	});

	it('omits a string that was already empty', () => {
		expect(guestPatch(guest({ email: undefined }), form({ email: '' }))).toEqual({});
	});

	it('includes cleared nullable fields as null', () => {
		expect(guestPatch(guest(), form({ angbaoAmt: '', giftItem: '' }))).toEqual({
			angbaoAmt: null,
			giftItem: null,
		});
	});

	it('accepts numeric angbao values that a number input binds', () => {
		// `type="number"` makes Svelte bind a number, not a string.
		expect(guestPatch(guest({ angbaoAmount: 50 }), form({ angbaoAmt: 200 as unknown as string }))).toEqual({ angbaoAmt: 200 });
		expect(guestPatch(guest({ angbaoAmount: 50 }), form({ angbaoAmt: 0 as unknown as string }))).toEqual({ angbaoAmt: 0 });
		expect(guestPatch(guest({ angbaoAmount: 50 }), form({ angbaoAmt: null as unknown as string }))).toEqual({ angbaoAmt: null });
	});

	it('includes changed fields', () => {
		expect(
			guestPatch(
				guest(),
				form({ name: 'Alice Tan', pax: 3, rsvp: 'confirmed', isVip: true, dietary: ['vegetarian', 'no nuts'], angbaoAmt: '200', giftItem: 'Vase' })
			)
		).toEqual({
			name: 'Alice Tan',
			pax: 3,
			rsvp: 'confirmed',
			isVip: true,
			dietary: ['vegetarian', 'no nuts'],
			angbaoAmt: 200,
			giftItem: 'Vase',
		});
	});
});
