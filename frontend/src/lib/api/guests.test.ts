import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('#lib/stores/index.ts', () => ({
	selectedGuest: { set: vi.fn(), update: vi.fn(), subscribe: (fn: (v: null) => void) => { fn(null); return () => {}; } },
	addToast: vi.fn()
}));
vi.mock('#lib/api/sse.ts', () => ({ connectSSE: vi.fn(), disconnectSSE: vi.fn() }));
vi.mock('#lib/api/client.ts', () => ({ apiFetch: vi.fn(), isNetworkError: (e: unknown) => e instanceof TypeError }));

const store = new Map<string, string>();

beforeEach(() => {
	store.clear();
	vi.stubGlobal('localStorage', { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) });
	vi.stubGlobal('window', { dispatchEvent: () => true });
	vi.stubGlobal('navigator', { onLine: false });
});
afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

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

	it('does not seed the staleness watermark from an offline-create local clock', async () => {
		const { createGuest } = await import('./guests');
		const { applyGuestResponse, guestMap } = await import('#lib/stores/guestEvents.ts');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockRejectedValue(new TypeError('Failed to fetch'));
		vi.useFakeTimers({ now: new Date('2030-01-01T00:00:00Z') });

		const created = await createGuest('w1', { name: 'A', pax: 2, rsvp: 'pending', isVip: false, notes: '', dietary: [] });
		// A server row stamped with real time must still apply after that fast clock.
		applyGuestResponse({ ...created, name: 'Server', updatedAt: '2026-01-01T00:00:00Z' });

		expect(get(guestMap).get(created.id)?.name).toBe('Server');
	});
});

describe('server rejection reason', () => {
	it('keeps the API title on a create failure', async () => {
		vi.stubGlobal('navigator', { onLine: true });
		const { createGuest } = await import('./guests');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockResolvedValue({
			ok: false, status: 400,
			json: async () => ({ title: 'seat overlaps with another guest: "Bob" (seats 3-4)' })
		} as Response);

		await expect(createGuest('w1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [] }))
			.rejects.toThrow('seat overlaps with another guest');
	});
});
