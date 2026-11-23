import { describe, expect, it } from 'vitest';
import { clampTableCapacity, groupGuestsByTable, highestOccupiedSeat } from './seat.ts';

describe('highestOccupiedSeat', () => {
	it('is the highest seat end, not the summed pax', () => {
		// Seats 1 and 8 with pax 2 each: sum is 4, the seat end is 9.
		const guests = [
			{ seatNumber: 1, pax: 2 },
			{ seatNumber: 8, pax: 2 }
		];
		expect(highestOccupiedSeat(guests)).toBe(9);
	});

	it('treats a missing pax as one seat', () => {
		expect(highestOccupiedSeat([{ seatNumber: 1 }, { seatNumber: 8 }])).toBe(8);
	});

	it('ignores guests without a seat', () => {
		expect(highestOccupiedSeat([{ seatNumber: null }, { seatNumber: 8, pax: 2 }])).toBe(9);
		expect(highestOccupiedSeat([])).toBe(0);
	});
});

describe('groupGuestsByTable', () => {
	it('groups seated guests and drops unassigned ones', () => {
		const grouped = groupGuestsByTable([
			{ id: 'a', tableId: 't1', seatNumber: 8, pax: 2 },
			{ id: 'b', tableId: null, seatNumber: null, pax: 1 },
			{ id: 'c', tableId: 't2', seatNumber: 1, pax: 1 }
		]);
		expect(grouped.t1.map((g) => g.id)).toEqual(['a']);
		expect(grouped.t2.map((g) => g.id)).toEqual(['c']);
		expect(grouped).not.toHaveProperty('null');
		// The layout editor's floor comes from this grouping.
		expect(highestOccupiedSeat(grouped.t1)).toBe(9);
	});
});

describe('clampTableCapacity', () => {
	it('clamps to the backend cap and the seated floor', () => {
		expect(clampTableCapacity(999, 1)).toBe(200);
		expect(clampTableCapacity(2, 8)).toBe(8);
		expect(clampTableCapacity(Number.NaN, 3)).toBe(3);
		expect(clampTableCapacity(12.9, 1)).toBe(12);
	});
});
