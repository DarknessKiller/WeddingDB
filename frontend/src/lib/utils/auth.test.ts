import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/api/client.ts', () => ({ refreshIfNeeded: vi.fn() }));
vi.mock('#lib/api/sse.ts', () => ({ connectSSE: vi.fn(), disconnectSSE: vi.fn() }));

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		clear: () => map.clear(),
		get length() { return map.size; },
		key: (i: number) => [...map.keys()][i] ?? null,
	};
}

beforeEach(() => {
	vi.resetModules();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
});

afterEach(() => {
	vi.unstubAllGlobals();
	delete (globalThis as any).localStorage;
});

// A rejected token with no refresh token ends the session; the next account
// must not find this wedding's guest rows cached on the device.
describe('validateToken session end', () => {
	it('clears the guest stores and offline caches', async () => {
		const { setAuth } = await import('#lib/stores/index.ts');
		const { seedGuests, guestList } = await import('#lib/stores/guestEvents.ts');
		const { toGuest } = await import('#lib/utils/guestTransform.ts');
		setAuth('dead-access', '', 'admin', 'A');
		seedGuests([toGuest({ id: 'g1', name: 'A', email: 'a@b.c', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: null, angbaoAmt: 88, giftItem: null })]);
		localStorage.setItem('offline_cache_w1', '[{"email":"a@b.c","angbaoAmount":88}]');

		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401 } as Response));
		const { validateToken } = await import('./auth.ts');
		expect(await validateToken()).toBe(false);

		expect(localStorage.getItem('offline_cache_w1')).toBeNull();
		expect(guestList).toBeDefined();
		const { get } = await import('svelte/store');
		expect(get(guestList)).toEqual([]);
	});
});
