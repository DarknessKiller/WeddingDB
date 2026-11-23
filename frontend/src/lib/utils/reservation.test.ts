import { describe, it, expect } from 'vitest';
import { reservationSchema } from './reservation';
import { LIMITS } from './limits';

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
