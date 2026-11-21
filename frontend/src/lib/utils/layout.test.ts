import { describe, it, expect } from 'vitest';
import { defaultSlot, layoutTablesPayload, reorderElements } from './layout';
import type { BanquetTable, HallElement } from '#lib/types/index.ts';

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
	// The server refuses X/Y outside 0–100, and one such table blocks every
	// later layout save, so a full grid must not hand out y > 100.
	it('stays inside 0–100 when every grid slot is taken', () => {
		const tables: BanquetTable[] = [];
		for (let i = 0; i < 40; i++) {
			const slot = defaultSlot(tables);
			expect(slot.y).toBeLessThanOrEqual(100);
			expect(slot.x).toBeLessThanOrEqual(100);
			tables.push(t(slot.x, slot.y));
		}
	});
	it('gives 60 tables distinct slots inside 0–100', () => {
		const tables: BanquetTable[] = [];
		for (let i = 0; i < 60; i++) tables.push(t(defaultSlot(tables).x, defaultSlot(tables).y));
		const keys = new Set(tables.map((x) => `${Math.round(x.x)},${Math.round(x.y)}`));
		expect(keys.size).toBe(60);
		for (const x of tables) {
			expect(x.x).toBeGreaterThanOrEqual(0);
			expect(x.x).toBeLessThanOrEqual(100);
			expect(x.y).toBeGreaterThanOrEqual(0);
			expect(x.y).toBeLessThanOrEqual(100);
		}
	});
});

describe('layoutTablesPayload', () => {
	it('sends capacity and isVip only for tables whose values the editor changed', () => {
		const a = { ...t(10, 10), id: 'a' };
		const b = { ...t(20, 20), id: 'b' };
		const c = { ...t(30, 30), id: 'c' };
		const edited = [{ ...a, x: 15 }, { ...b, capacity: 12 }, { ...c, isVip: true }, { ...t(40, 40), id: 'new' }];

		expect(layoutTablesPayload(edited, [a, b, c])).toEqual([
			{ id: 'a', x: 15, y: 10, degree: 0 },
			{ id: 'b', x: 20, y: 20, degree: 0, capacity: 12 },
			{ id: 'c', x: 30, y: 30, degree: 0, isVip: true },
			{ id: 'new', x: 40, y: 40, degree: 0, capacity: 10, isVip: false }
		]);
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
