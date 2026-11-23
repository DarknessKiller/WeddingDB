import type { BanquetTable, HallElement } from '#lib/types/index.ts';

// Hall size bounds the layout editor enforces before saving: the API only
// rejects non-positive values, so a typo must not reach it.
export const HALL_MIN = 200;
export const HALL_MAX = 3000;

export function clampHallSize(value: number): number {
	const v = Math.floor(Number(value));
	return Math.min(HALL_MAX, Math.max(HALL_MIN, Number.isFinite(v) ? v : HALL_MIN));
}

// ponytail: fixed 5-col default grid; drag exists for everything else
export function defaultSlot(tables: BanquetTable[]): { x: number; y: number } {
	const taken = new Set(tables.map((t) => `${Math.round(t.x)},${Math.round(t.y)}`));
	for (let row = 1; row <= 20; row++) {
		for (let col = 1; col <= 5; col++) {
			const x = (100 / 6) * col;
			const y = 15 * row;
			if (!taken.has(`${Math.round(x)},${Math.round(y)}`)) {
				return { x, y };
			}
		}
	}
	return { x: 50, y: 50 };
}

export function reorderElements(elements: HallElement[], id: string, direction: 'front' | 'back'): HallElement[] {
	const selected = elements.find((element) => element.id === id);
	if (!selected) return elements;
	const others = elements.filter((element) => element.id !== id);
	const ordered = direction === 'front' ? [...others, selected] : [selected, ...others];
	return ordered.map((element, zIndex) => ({ ...element, zIndex: zIndex * 10 }));
}
