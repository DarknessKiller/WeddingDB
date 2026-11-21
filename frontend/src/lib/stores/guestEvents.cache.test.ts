import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { GuestResponse } from '#lib/api/guests.ts';

vi.mock('#lib/stores/index.ts', () => ({
	selectedGuest: { set: vi.fn(), update: vi.fn(), subscribe: (cb: (v: null) => void) => { cb(null); return () => {}; } }
}));
vi.mock('#lib/api/sse.ts', () => ({
	connectSSE: () => ({ onEvent: () => () => {}, onStatus: () => () => {} }),
	disconnectSSE: () => {}
}));
vi.mock('#lib/stores/weddingId.ts', () => ({
	weddingId: { subscribe: (cb: (v: string) => void) => { cb('w1'); return () => {}; } }
}));

function resp(id: string): GuestResponse {
	return {
		id, weddingId: 'w1', name: id, email: '', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [],
		tableId: null, seatNum: null, checkedInAt: '2026-01-01T10:00:00Z', angbaoAmt: 88, giftItem: null,
		createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T10:00:00Z'
	};
}

beforeEach(() => {
	vi.resetModules();
	const map = new Map<string, string>();
	Object.defineProperty(globalThis, 'sessionStorage', {
		value: { getItem: (k: string) => map.get(k) ?? null, setItem: (k: string, v: string) => void map.set(k, v), removeItem: (k: string) => void map.delete(k) },
		configurable: true
	});
});

afterEach(() => {
	vi.useRealTimers();
});


// An offline reload reads the session cache, so a change made after the
// roster loaded must reach it; a partial list before the load must not.
describe('offline roster cache', () => {
	it('re-caches the roster after a change once it is seeded', async () => {
		vi.useFakeTimers();
		const { seedGuests, applyGuestResponse } = await import('./guestEvents');
		const { loadCachedGuests } = await import('#lib/offline/queue.ts');
		seedGuests([]);

		applyGuestResponse(resp('g1'));

		// One trailing write per burst: nothing is serialized per event.
		expect(loadCachedGuests('w1')).toEqual([]);
		vi.advanceTimersByTime(500);
		expect(loadCachedGuests<{ id: string; angbaoAmount: number }>('w1')).toMatchObject([{ id: 'g1', angbaoAmount: 88 }]);
	});

	it('leaves the cache alone before the roster is seeded', async () => {
		const { clearGuests, applyGuestResponse } = await import('./guestEvents');
		const { cacheGuests, loadCachedGuests } = await import('#lib/offline/queue.ts');
		cacheGuests('w1', [{ id: 'full-roster' }]);
		clearGuests();

		applyGuestResponse(resp('g1'));

		expect(loadCachedGuests<{ id: string }>('w1')).toEqual([{ id: 'full-roster' }]);
	});
});
