import type { BanquetTable, HallElement } from '#lib/types/index.ts';

// ponytail: fixed 5-col default grid, then a second grid offset by half a step;
// drag exists for everything else. All slots stay within the 0–100 the server accepts.
export function defaultSlot(tables: BanquetTable[]): { x: number; y: number } {
	const taken = new Set(tables.map((t) => `${Math.round(t.x)},${Math.round(t.y)}`));
	const free = (x: number, y: number) => !taken.has(`${Math.round(x)},${Math.round(y)}`);
	for (let row = 1; row <= 6; row++) { // 5x6 grid: x = 16.7..83.3, y = 15..90
		for (let col = 1; col <= 5; col++) {
			const x = (100 / 6) * col;
			const y = 15 * row;
			if (free(x, y)) return { x, y };
		}
	}
	for (let row = 0; row <= 6; row++) { // 6x7 offset grid: x = 8.3..91.7, y = 7.5..97.5
		for (let col = 0; col <= 5; col++) {
			const x = 100 / 12 + (100 / 6) * col;
			const y = 7.5 + 15 * row;
			if (free(x, y)) return { x, y };
		}
	}
	return { x: 50, y: 50 };
}

// Layout save rows: capacity/isVip go out only when the editor changed them,
// so a layout save never overwrites a capacity or VIP flag set elsewhere meanwhile.
// `loaded` is the table list the editor started from.
export function layoutTablesPayload(edited: BanquetTable[], loaded: BanquetTable[]) {
	const before = new Map(loaded.map((t) => [t.id, t]));
	return edited.map(({ id, x, y, degree, capacity, isVip }) => {
		const b = before.get(id);
		return { id, x, y, degree, ...(b?.capacity !== capacity ? { capacity } : {}), ...(b?.isVip !== isVip ? { isVip } : {}) };
	});
}

export function reorderElements(elements: HallElement[], id: string, direction: 'front' | 'back'): HallElement[] {
	const selected = elements.find((element) => element.id === id);
	if (!selected) return elements;
	const others = elements.filter((element) => element.id !== id);
	const ordered = direction === 'front' ? [...others, selected] : [selected, ...others];
	return ordered.map((element, zIndex) => ({ ...element, zIndex: zIndex * 10 }));
}
