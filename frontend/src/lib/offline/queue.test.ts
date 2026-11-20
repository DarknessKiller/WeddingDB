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

describe('queue limits', () => {
	it('refuses new edits when full instead of dropping the oldest', async () => {
		const { enqueue, getQueue } = await import('./queue');
		for (let i = 0; i < 500; i++) {
			enqueue('w1', {
				mutationId: `m${i}`,
				op: 'update',
				guestId: 'g1',
				clientUpdatedAt: '2026-01-01T00:00:00Z'
			});
		}

		expect(() =>
			enqueue('w1', {
				mutationId: 'overflow',
				op: 'update',
				guestId: 'g1',
				clientUpdatedAt: '2026-01-01T00:00:00Z'
			})
		).toThrow(/full/);

		const q = getQueue('w1');
		expect(q).toHaveLength(500);
		expect(q[0].mutationId).toBe('m0');
	});

	it('drops a mutation after repeated failed syncs', async () => {
		localStorage.setItem(
			'offline_queue_w1',
			JSON.stringify([
				{ mutationId: 'm1', op: 'update', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z', attempts: 4 }
			])
		);
		apiFetch.mockResolvedValue({
			ok: true,
			json: async () => ({ results: [{ guestId: 'g1', status: 'failed', reason: 'invalid pax' }] })
		});

		const { syncQueue, getQueue } = await import('./queue');
		await syncQueue('w1');

		expect(getQueue('w1')).toEqual([]);
	});

	it('keeps a failed mutation and counts the attempt below the limit', async () => {
		localStorage.setItem(
			'offline_queue_w1',
			JSON.stringify([
				{ mutationId: 'm1', op: 'update', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' }
			])
		);
		apiFetch.mockResolvedValue({
			ok: true,
			json: async () => ({ results: [{ guestId: 'g1', status: 'failed', reason: 'invalid pax' }] })
		});

		const { syncQueue, getQueue } = await import('./queue');
		await syncQueue('w1');

		const q = getQueue('w1');
		expect(q).toHaveLength(1);
		expect(q[0].attempts).toBe(1);
	});
});
