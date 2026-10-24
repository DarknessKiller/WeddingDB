import type { Guest } from '$lib/types';

// Local guest search over the client-side guest list: instant, works offline.
// Scoring mirrors the API order: name startsWith, then pinyin prefix, then contains.
// Requires Guest.namePinyin from the API (backend generates it in Go).
export function searchGuestsLocal(guests: Guest[], q: string, limit = 50): Guest[] {
	const query = q.trim().toLowerCase();
	if (!query) return [];
	const nameQ = query.toLowerCase();
	const pyQ = query.replace(/\s+/g, '').toLowerCase();
	const scored: { g: Guest; s: number }[] = [];
	for (const g of guests) {
		const name = (g.name || '').toLowerCase();
		const py = (g.namePinyin || '').toLowerCase();
		let s = -1;
		if (name.startsWith(nameQ)) s = 3;
		else if (name.includes(nameQ)) s = 2;
		if (py && pyQ) {
			if (py.startsWith(pyQ)) s = Math.max(s, 2);
			else if (py.includes(pyQ)) s = Math.max(s, 1);
		}
		if (s >= 0) scored.push({ g, s });
	}
	return scored.sort((a, b) => b.s - a.s || a.g.name.localeCompare(b.g.name)).slice(0, limit).map(x => x.g);
}
