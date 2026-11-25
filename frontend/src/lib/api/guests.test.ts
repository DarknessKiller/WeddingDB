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

	it('refuses an edit when the queue is full and keeps every queued one', async () => {
		const { seedGuests, guestMap } = await import('#lib/stores/guestEvents.ts');
		const { updateGuest } = await import('./guests');
		const { toGuest } = await import('#lib/utils/guestTransform.ts');
		seedGuests([toGuest({ id: 'g1', name: 'A', email: '', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [], tableId: null, seatNum: null, checkedInAt: null, angbaoAmt: null, giftItem: null })]);
		const full = Array.from({ length: 500 }, (_, i) => ({ mutationId: `m${i}`, op: 'update', guestId: 'g0', clientUpdatedAt: '2030-01-01T00:00:00Z' }));
		store.set('offline_queue_w1', JSON.stringify(full));

		await expect(updateGuest('w1', 'g1', { name: 'B' })).rejects.toThrow(/queue full/);

		expect(JSON.parse(store.get('offline_queue_w1')!)).toEqual(full);
		expect(get(guestMap).get('g1')?.name).toBe('A');
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

	it('keeps the API title on a delete failure', async () => {
		vi.stubGlobal('navigator', { onLine: true });
		const { deleteGuest } = await import('./guests');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockResolvedValue({
			ok: false, status: 400,
			json: async () => ({ title: 'guest has a queued edit' })
		} as Response);

		await expect(deleteGuest('w1', 'g1')).rejects.toThrow('guest has a queued edit');
	});
});

describe('lost response retry', () => {
	it('sends one client UUID with the create and reuses it for the queued fallback', async () => {
		vi.stubGlobal('navigator', { onLine: true });
		// No randomUUID (plain-HTTP origin): the fallback must still be a v4 UUID.
		const real = globalThis.crypto;
		vi.stubGlobal('crypto', { getRandomValues: (a: Uint8Array<ArrayBuffer>) => real.getRandomValues(a) });
		const { createGuest } = await import('./guests');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockRejectedValue(new TypeError('Failed to fetch'));

		const created = await createGuest('w1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [] });

		const sent = JSON.parse(vi.mocked(apiFetch).mock.calls.at(-1)![1]!.body as string);
		expect(sent.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
		expect(created.id).toBe(sent.id);
		expect(JSON.parse(store.get('offline_queue_w1')!)[0].guestId).toBe(sent.id);
	});

	it('queues a failed online check-in with the time stamped before the attempt', async () => {
		vi.stubGlobal('navigator', { onLine: true });
		vi.useFakeTimers({ now: new Date('2030-01-01T00:00:00Z') });
		const { checkInGuest } = await import('./guests');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockImplementation(async () => {
			// The server stamped its check-in, then the response was lost.
			vi.setSystemTime(new Date('2030-01-01T00:00:05Z'));
			throw new TypeError('Failed to fetch');
		});

		await checkInGuest('w1', 'g1', 'late');

		expect(JSON.parse(store.get('offline_queue_w1')!)[0].clientUpdatedAt).toBe('2030-01-01T00:00:00.000Z');
	});

	it.each([
		['createGuest', (m: typeof import('./guests')) => m.createGuest('w1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [] })],
		['updateGuest', (m: typeof import('./guests')) => m.updateGuest('w1', 'g1', { name: 'B' })],
		['deleteGuest', (m: typeof import('./guests')) => m.deleteGuest('w1', 'g1')],
		['checkOutGuest', (m: typeof import('./guests')) => m.checkOutGuest('w1', 'g1')],
		['assignSeat', (m: typeof import('./guests')) => m.assignSeat('w1', 'g1', 't1', 1)],
		['unassignSeat', (m: typeof import('./guests')) => m.unassignSeat('w1', 'g1')],
	])('%s queues a timed-out attempt with the time stamped before it', async (_name, call) => {
		vi.stubGlobal('navigator', { onLine: true });
		vi.useFakeTimers({ now: new Date('2030-01-01T00:00:00Z') });
		const mod = await import('./guests');
		const { apiFetch } = await import('#lib/api/client.ts');
		vi.mocked(apiFetch).mockImplementation(async () => {
			// The request landed, then the 10s client timeout fired.
			vi.setSystemTime(new Date('2030-01-01T00:00:10Z'));
			throw new TypeError('Failed to fetch');
		});

		await call(mod);

		expect(JSON.parse(store.get('offline_queue_w1')!)[0].clientUpdatedAt).toBe('2030-01-01T00:00:00.000Z');
	});
});
