import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('#lib/stores/index.ts', async () => {
	const { writable } = await import('svelte/store');
	return { selectedGuest: writable(null) };
});
vi.mock('#lib/api/sse.ts', () => ({ connectSSE: vi.fn(), disconnectSSE: vi.fn() }));

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k)
	};
}

function guest(name: string) {
	return {
		id: name, name, email: '', rsvp: 'confirmed', pax: 1, tableId: null,
		checkedIn: false, dietaryRequirements: [], isVip: false,
		createdAt: new Date(0), updatedAt: new Date(0)
	} as any;
}

beforeEach(() => {
	vi.resetModules();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
});

afterEach(() => {
	delete (globalThis as any).localStorage;
});

describe('refreshGuests wedding identity', () => {
	it('a stale fetch for the old wedding does not overwrite the new wedding list', async () => {
		const { setWeddingId } = await import('#lib/stores/weddingId.ts');
		const { guestList, refreshGuests, seedGuests } = await import('./guestEvents.ts');
		setWeddingId('w1');
		let releaseOld!: (guests: ReturnType<typeof guest>[]) => void;

		const oldRun = refreshGuests(() => new Promise((r) => { releaseOld = r as any; }), 'w1');
		await new Promise((r) => setTimeout(r, 0)); // let the w1 fetch start
		// Layout switched to w2 and its snapshot already landed.
		setWeddingId('w2');
		seedGuests([guest('new')]);
		releaseOld([guest('old')]);
		await oldRun;

		expect(get(guestList).map((g) => g.name)).toEqual(['new']);
	});
});
