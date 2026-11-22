import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('#lib/stores/index.ts', () => ({
	selectedGuest: { set: vi.fn(), update: vi.fn(), subscribe: (fn: (v: null) => void) => { fn(null); return () => {}; } },
	addToast: vi.fn()
}));
vi.mock('#lib/api/sse.ts', () => ({ connectSSE: vi.fn(), disconnectSSE: vi.fn() }));
vi.mock('#lib/api/client.ts', () => ({ apiFetch: vi.fn() }));

const store = new Map<string, string>();

beforeEach(() => {
	store.clear();
	vi.stubGlobal('localStorage', { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) });
	vi.stubGlobal('window', { dispatchEvent: () => true });
	vi.stubGlobal('navigator', { onLine: false });
});
afterEach(() => vi.unstubAllGlobals());

describe('offline optimistic patch', () => {
	it('updates guestMap and occupancy, not only guestList', async () => {
		const { seedGuests, guestMap, tableOccupancy } = await import('#lib/stores/guestEvents.ts');
		const { assignSeat, checkInGuest } = await import('./guests');
		const { toGuest } = await import('#lib/utils/guestTransform.ts');
		seedGuests([toGuest({ id: 'g1', name: 'A', email: '', pax: 2, rsvp: 'pending', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: null, angbaoAmt: null, giftItem: null })]);

		await assignSeat('w1', 'g1', 't1', 4);
		await checkInGuest('w1', 'g1', 'late');

		expect(get(guestMap).get('g1')).toMatchObject({ tableId: 't1', seatNumber: 4, checkedIn: true, notes: 'late' });
		expect(get(tableOccupancy).get('t1')).toBe(2);
		expect(JSON.parse(store.get('offline_queue_w1')!)).toHaveLength(2);
	});
});
