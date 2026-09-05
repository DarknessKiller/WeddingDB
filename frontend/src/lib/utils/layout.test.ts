import { describe, it, expect } from 'vitest';
import { defaultSlot, reorderElements } from './layout';
import type { BanquetTable, HallElement } from '$lib/types';

const t = (x: number, y: number): BanquetTable => ({
	id: crypto.randomUUID(), name: '', capacity: 10, x, y, degree: 0, isVip: false
});

const el = (id: string, zIndex: number): HallElement => ({
	id, type: 'box', x: 50, y: 50, degree: 0, width: 10, height: 10,
	name: '', color: '', textColor: '', strokeColor: '', opacity: 1, zIndex
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

describe('reorderElements', () => {
	const elements = [el('a', 0), el('b', 10), el('c', 20)];

	it('moves the selected element to either edge and renumbers z-indexes', () => {
		expect(reorderElements(elements, 'b', 'front').map(({ id, zIndex }) => ({ id, zIndex })))
			.toEqual([{ id: 'a', zIndex: 0 }, { id: 'c', zIndex: 10 }, { id: 'b', zIndex: 20 }]);
		expect(reorderElements(elements, 'b', 'back').map(({ id, zIndex }) => ({ id, zIndex })))
			.toEqual([{ id: 'b', zIndex: 0 }, { id: 'a', zIndex: 10 }, { id: 'c', zIndex: 20 }]);
	});
});
