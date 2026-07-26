import { describe, it, expect } from 'vitest';
import { defaultSlot } from './layout';
import type { BanquetTable } from '$lib/types';

const t = (x: number, y: number): BanquetTable => ({
	id: crypto.randomUUID(), name: '', capacity: 10, x, y, degree: 0, isVip: false
});

describe('defaultSlot', () => {
	it('returns first slot when empty', () => {
		const slot = defaultSlot([]);
		expect(slot.x).toBeCloseTo(100 / 6, 5);
		expect(slot.y).toBeCloseTo(15, 5);
	});
	it('advances along the row', () => {
		const slot = defaultSlot([t(100 / 6, 15)]);
		expect(slot.x).toBeCloseTo((100 / 6) * 2, 5);
		expect(slot.y).toBeCloseTo(15, 5);
	});
	it('wraps to next row after 5 columns', () => {
		const tables = [
			t(100 / 6, 15), t((100 / 6) * 2, 15), t((100 / 6) * 3, 15),
			t((100 / 6) * 4, 15), t((100 / 6) * 5, 15)
		];
		const slot = defaultSlot(tables);
		expect(slot.x).toBeCloseTo(100 / 6, 5);
		expect(slot.y).toBeCloseTo(30, 5);
	});
});
