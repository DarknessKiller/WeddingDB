import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.fn();
const enqueue = vi.fn();

vi.mock('./client', () => ({ apiFetch: (...args: unknown[]) => apiFetch(...args) }));
vi.mock('#lib/offline/queue.ts', () => ({ enqueue: (...args: unknown[]) => enqueue(...args), syncQueue: vi.fn() }));
vi.mock('#lib/stores/guestEvents.ts', () => ({ applyGuestResponse: vi.fn(), guestList: { update: vi.fn() } }));

const realNavigator = globalThis.navigator;

beforeEach(() => {
	apiFetch.mockReset();
	enqueue.mockReset();
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
