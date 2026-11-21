import { describe, expect, it } from 'vitest';
import { firstFreeSeatRun, isContiguous } from './seat';

describe('firstFreeSeatRun', () => {
	it('skips a gap too small for the party', () => {
		expect(firstFreeSeatRun(10, new Set([2, 3]), 3)).toEqual([4, 5, 6]);
	});

	it('returns no seats when no run fits', () => {
		expect(firstFreeSeatRun(5, new Set([2, 4]), 2)).toEqual([]);
	});

	it('starts at seat 1 when the table is empty', () => {
		expect(firstFreeSeatRun(10, new Set(), 2)).toEqual([1, 2]);
	});
});

describe('isContiguous', () => {
	it('rejects scattered seats', () => {
		expect(isContiguous([1, 4, 5])).toBe(false);
		expect(isContiguous([4, 5, 6])).toBe(true);
	});
});
