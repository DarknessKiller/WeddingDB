import { describe, it, expect } from 'vitest';
import { reservationSchema } from './reservation';
import { LIMITS } from './limits';
import { seatSelectionError, closestEmptySeats, seatOccupant } from './reservation.ts';

const base = { name: 'Alice', pax: 1, rsvp: 'pending' as const };

describe('reservationSchema pax', () => {
	it('accepts the API party-size cap', () => {
		expect(reservationSchema.safeParse({ ...base, pax: LIMITS.guestPax }).success).toBe(true);
	});

	it('rejects one more guest than the API accepts', () => {
		const result = reservationSchema.safeParse({ ...base, pax: LIMITS.guestPax + 1 });
		expect(result.success).toBe(false);
	});
});

describe('seatSelectionError', () => {
	it('requires seats when a table is chosen', () => {
		expect(seatSelectionError({ tableId: 't1', pax: 2 })).toBe('Select at least one seat');
		expect(seatSelectionError({ tableId: 't1', seatNumbers: [], pax: 2 })).toBe('Select at least one seat');
	});

	it('requires one seat per guest', () => {
		expect(seatSelectionError({ tableId: 't1', seatNumbers: [3], pax: 2 })).toContain('Select 2 seat(s)');
	});

	it('allows an unseated reservation or a full selection', () => {
		expect(seatSelectionError({ pax: 2 })).toBeUndefined();
		expect(seatSelectionError({ tableId: 't1', seatNumbers: [3, 4], pax: 2 })).toBeUndefined();
	});
});

describe('closestEmptySeats', () => {
	it('returns a contiguous run so the server assignment matches the shown seats', () => {
		expect(closestEmptySeats(10, new Set([2]), 2)).toEqual([3, 4]);
	});

	it('uses the first free seat for one guest', () => {
		expect(closestEmptySeats(10, new Set([1]), 1)).toEqual([2]);
	});

	it('returns nothing when the party does not fit', () => {
		expect(closestEmptySeats(3, new Set([1, 3]), 2)).toEqual([]);
	});
});

describe('seatOccupant', () => {
	const guests = [
		{ tableId: 't1', seatNumber: 4, pax: 3 },
		{ tableId: 't2', seatNumber: 1, pax: 1 },
	];

	it('finds the occupant of any seat in the booked range', () => {
		expect(seatOccupant(guests, 't1', 4)).toBe(guests[0]);
		expect(seatOccupant(guests, 't1', 6)).toBe(guests[0]);
	});

	it('reports nothing outside the range or on another table', () => {
		expect(seatOccupant(guests, 't1', 3)).toBeUndefined();
		expect(seatOccupant(guests, 't1', 7)).toBeUndefined();
		expect(seatOccupant(guests, 't2', 5)).toBeUndefined();
	});
});

describe('guestFormErrors', () => {
	it('blocks the values the guest API rejects, with a message per field', async () => {
		const { guestFormErrors } = await import('./reservation.ts');
		expect(guestFormErrors({ name: 'Alice', pax: 2, angbaoAmt: '' })).toEqual({});
		expect(guestFormErrors({ name: 'Alice', pax: 1, angbaoAmt: null })).toEqual({});
		expect(guestFormErrors({ name: 'Alice', pax: LIMITS.guestPax, angbaoAmt: 0 })).toEqual({});
		const bad = guestFormErrors({ name: '   ', pax: 1.5, angbaoAmt: -1 });
		expect(bad.name?.[0]).toBe('Name is required');
		expect(bad.pax?.[0]).toBe('Pax must be a whole number');
		expect(bad.angbaoAmt?.[0]).toBe('Angbao must be a whole number from 0 to 10,000,000');
		expect(guestFormErrors({ name: 'A', pax: null, angbaoAmt: 1.5 })).toMatchObject({ pax: ['Pax must be a whole number'], angbaoAmt: ['Angbao must be a whole number from 0 to 10,000,000'] });
		expect(guestFormErrors({ name: 'A', pax: 1, angbaoAmt: 10_000_001 }).angbaoAmt?.[0]).toBe('Angbao must be a whole number from 0 to 10,000,000');
		expect(guestFormErrors({ name: 'A', pax: LIMITS.guestPax + 1, angbaoAmt: '' }).pax?.[0]).toBe(`Max ${LIMITS.guestPax} guests`);
	});

	it('accepts an empty or bare email and rejects what services.ValidGuestEmail refuses', async () => {
		const { guestFormErrors } = await import('./reservation.ts');
		for (const email of ['', 'tan.ah-kow+rsvp@mail.example.sg']) {
			expect(guestFormErrors({ name: 'A', pax: 1, angbaoAmt: '', email })).toEqual({});
		}
		for (const email of ['not-an-email', 'Tan <tan@x.io>', ' tan@x.io']) {
			expect(guestFormErrors({ name: 'A', pax: 1, angbaoAmt: '', email }).email?.[0]).toBe('Invalid email');
		}
	});
});
