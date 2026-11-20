import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		clear: () => map.clear()
	};
}

const apiFetch = vi.fn();

vi.mock('#lib/api/client.ts', () => ({
	apiFetch: (...args: unknown[]) => apiFetch(...args)
}));

beforeEach(() => {
	vi.resetModules();
	apiFetch.mockReset();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
	Object.defineProperty(globalThis, 'window', {
		value: { dispatchEvent: () => {}, addEventListener: () => {} },
		configurable: true
	});
});

afterEach(() => {
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
});

describe('syncQueue', () => {
	it('sends one request for overlapping sync calls and dequeues once', async () => {
		localStorage.setItem(
			'offline_queue_w1',
			JSON.stringify([{ mutationId: 'm1', op: 'checkin', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' }])
		);
		let release: () => void = () => {};
		apiFetch.mockImplementation(
			() =>
				new Promise((resolve) => {
					release = () =>
						resolve({ ok: true, json: async () => ({ results: [{ guestId: 'g1', status: 'applied' }] }) });
				})
		);

		const { syncQueue, getQueue } = await import('./queue');
		const first = syncQueue('w1');
		const second = syncQueue('w1');

		expect(apiFetch).toHaveBeenCalledOnce();
		release();
		const [a, b] = await Promise.all([first, second]);

		expect(apiFetch).toHaveBeenCalledOnce();
		expect(a).toEqual(b);
		expect(getQueue('w1')).toEqual([]);
	});
});
