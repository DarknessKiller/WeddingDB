import type { BanquetTable } from '$lib/types';

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
