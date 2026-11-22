import { describe, it, expect } from 'vitest';
import { firstFittingSeat } from './seats';

describe('firstFittingSeat', () => {
	it('returns the lowest free seat', () => {
		expect(firstFittingSeat([1, 2, 3], new Set(), 1, 3)).toBe(1);
	});

	it('skips seats blocked inside the party range', () => {
		// Seats 2 and 3 are taken, so a party of 2 cannot start at 1 or 2.
		expect(firstFittingSeat([1, 4, 5], new Set([2, 3]), 2, 5)).toBe(1 + 3);
	});

	it('returns one past capacity when nothing fits', () => {
		expect(firstFittingSeat([5], new Set(), 2, 5)).toBe(6);
	});
});
