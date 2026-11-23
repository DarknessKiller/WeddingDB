import { describe, it, expect } from 'vitest';
import { reservationSchema } from './reservation';
import { LIMITS } from './limits';
import { seatSelectionError } from './reservation.ts';

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
