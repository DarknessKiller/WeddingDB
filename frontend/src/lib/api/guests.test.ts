import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.fn();
const enqueue = vi.fn();
const getQueue = vi.fn((): { guestId: string }[] => []);

vi.mock('./client', () => ({ apiFetch: (...args: unknown[]) => apiFetch(...args) }));
vi.mock('#lib/offline/queue.ts', () => ({ enqueue: (...args: unknown[]) => enqueue(...args), getQueue: () => getQueue(), syncQueue: vi.fn() }));
vi.mock('#lib/stores/guestEvents.ts', () => ({ applyGuestResponse: vi.fn(), cacheGuestList: vi.fn(), guestList: { update: vi.fn() } }));

const realNavigator = globalThis.navigator;

beforeEach(() => {
	apiFetch.mockReset();
	enqueue.mockReset();
	getQueue.mockReturnValue([]);
	Object.defineProperty(globalThis, 'navigator', { value: { onLine: true }, configurable: true });
	vi.useFakeTimers();
	vi.setSystemTime(new Date('2026-01-01T10:00:00Z'));
});

afterEach(() => {
	vi.useRealTimers();
	Object.defineProperty(globalThis, 'navigator', { value: realNavigator, configurable: true });
});

// The request reaches the server, then the response is lost some time later.
function lostResponse() {
	apiFetch.mockImplementation(async () => {
		vi.setSystemTime(new Date('2026-01-01T10:00:30Z'));
		throw new TypeError('Failed to fetch');
	});
}

describe('guest mutations on network error', () => {
	it('queues the retry with the timestamp taken before the request', async () => {
		lostResponse();
		const { checkInGuest } = await import('./guests');

		await checkInGuest('w1', 'g1');

		expect(enqueue).toHaveBeenCalledOnce();
		expect(enqueue.mock.calls[0][1].clientUpdatedAt).toBe('2026-01-01T10:00:00.000Z');
	});

	it('does not queue a create whose request may have reached the server', async () => {
		lostResponse();
		const { createGuest } = await import('./guests');

		await expect(createGuest('w1', { name: 'A', pax: 1 } as never)).rejects.toThrow('Failed to fetch');
		expect(enqueue).not.toHaveBeenCalled();
	});
});

describe('updateGuest conflicts', () => {
	it('turns a 409 into ConflictError and does not queue the edit', async () => {
		apiFetch.mockResolvedValue(new Response(JSON.stringify({ title: 'Someone else changed this guest.' }), { status: 409 }));
		const { updateGuest, ConflictError } = await import('./guests');

		const p = updateGuest('w1', 'g1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [], baseUpdatedAt: '2026-01-01T09:00:00Z' });
		await expect(p).rejects.toBeInstanceOf(ConflictError);
		expect(JSON.parse(apiFetch.mock.calls[0][1].body).baseUpdatedAt).toBe('2026-01-01T09:00:00Z');
		expect(enqueue).not.toHaveBeenCalled();
	});
});

describe('offline ids', () => {
	it('gives an offline-created guest a base64url UUID id like the server uses', async () => {
		Object.defineProperty(globalThis, 'navigator', { value: { onLine: false }, configurable: true });
		apiFetch.mockRejectedValue(new TypeError('Failed to fetch'));
		const { createGuest } = await import('./guests');

		const g = await createGuest('w1', { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [] });
		expect(g.id).toMatch(/^[A-Za-z0-9_-]{22}$/);
		expect(enqueue.mock.calls[0][1].guestId).toBe(g.id);
	});
});

describe('queued edits carry the editor base', () => {
	const edit = { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [], baseUpdatedAt: '2026-01-01T09:00:00Z' } as never;

	it('sends the loaded updatedAt so the server can refuse a stale copy', async () => {
		apiFetch.mockRejectedValue(new TypeError('Failed to fetch'));
		const { updateGuest } = await import('./guests');

		await updateGuest('w1', 'g1', edit);

		expect(enqueue.mock.calls[0][1].baseUpdatedAt).toBe('2026-01-01T09:00:00Z');
	});

	it('drops the base when an earlier queued edit of the same guest is pending', async () => {
		apiFetch.mockRejectedValue(new TypeError('Failed to fetch'));
		getQueue.mockReturnValue([{ guestId: 'g1' }]);
		const { updateGuest } = await import('./guests');

		await updateGuest('w1', 'g1', edit);

		expect(enqueue.mock.calls[0][1].baseUpdatedAt).toBeNull();
	});
});

describe('server errors', () => {
	it('keeps the server reason for create, update and seat assign', async () => {
		apiFetch.mockImplementation(async () => new Response(JSON.stringify({ title: 'table full' }), { status: 400 }));
		const { createGuest, updateGuest, assignSeat } = await import('./guests');
		const data = { name: 'A', pax: 1, rsvp: 'pending', isVip: false, notes: '', dietary: [] } as never;

		await expect(createGuest('w1', data)).rejects.toThrow('table full');
		await expect(updateGuest('w1', 'g1', data)).rejects.toThrow('table full');
		await expect(assignSeat('w1', 'g1', 't1', 1)).rejects.toThrow('table full');
	});

	it('falls back to the status when the body has no reason', async () => {
		apiFetch.mockResolvedValue(new Response('oops', { status: 500 }));
		const { assignSeat } = await import('./guests');

		await expect(assignSeat('w1', 'g1', 't1', 1)).rejects.toThrow('Failed to assign seat: 500');
	});
});
