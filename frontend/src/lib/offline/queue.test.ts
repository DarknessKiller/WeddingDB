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
	it('lets a second tab send only what the first tab left', async () => {
		// A fake Web Locks API that runs same-name requests one after another.
		const tail = new Map<string, Promise<unknown>>();
		const locks = {
			request: (name: string, cb: () => Promise<unknown>) => {
				const run = (tail.get(name) ?? Promise.resolve()).then(cb);
				tail.set(name, run.catch(() => {}));
				return run;
			}
		};
		const realNavigator = globalThis.navigator;
		Object.defineProperty(globalThis, 'navigator', { value: { onLine: true, locks }, configurable: true });
		try {
			localStorage.setItem(
				'offline_queue_w1',
				JSON.stringify([{ mutationId: 'm1', op: 'checkin', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' }])
			);
			const pending: (() => void)[] = [];
			const release = () => pending.forEach((r) => r());
			apiFetch.mockImplementation(
				() =>
					new Promise((resolve) => {
						pending.push(() => resolve({ ok: true, json: async () => ({ results: [{ guestId: 'g1', status: 'applied' }] }) }));
					})
			);

			// Each import is a separate tab: its own module state, shared storage and locks.
			const tabA = await import('./queue');
			vi.resetModules();
			const tabB = await import('./queue');
			const a = tabA.syncQueue('w1');
			const b = tabB.syncQueue('w1');
			await Promise.resolve();
			release();
			await Promise.all([a, b]);

			expect(apiFetch).toHaveBeenCalledOnce();
			expect(tabB.getQueue('w1')).toEqual([]);
		} finally {
			Object.defineProperty(globalThis, 'navigator', { value: realNavigator, configurable: true });
		}
	});

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
		const { toasts } = await import('#lib/stores/index.ts');
		const { get } = await import('svelte/store');
		await syncQueue('w1');

		expect(getQueue('w1')).toEqual([]);
		expect(get(toasts).map(t => t.message)).toEqual([expect.stringMatching(/discarded/)]);
	});

	it('tells staff when a queued edit is skipped as older than the server', async () => {
		localStorage.setItem(
			'offline_queue_w1',
			JSON.stringify([
				{ mutationId: 'm1', op: 'update', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' }
			])
		);
		apiFetch.mockResolvedValue({
			ok: true,
			json: async () => ({ results: [{ guestId: 'g1', status: 'skipped', reason: 'older than server' }] })
		});

		const { syncQueue, getQueue } = await import('./queue');
		const { toasts } = await import('#lib/stores/index.ts');
		const { get } = await import('svelte/store');
		await syncQueue('w1');

		expect(getQueue('w1')).toEqual([]);
		expect(get(toasts).map(t => t.message)).toEqual([expect.stringMatching(/not applied/)]);
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

describe('sync retries', () => {
	it('keeps a retryable failure queued without spending an attempt', async () => {
		localStorage.setItem(
			'offline_queue_w1',
			JSON.stringify([
				{ mutationId: 'm1', op: 'checkin', guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' },
				{ mutationId: 'm2', op: 'update', guestId: 'g2', clientUpdatedAt: '2026-01-01T00:00:00Z' }
			])
		);
		apiFetch.mockResolvedValue({
			ok: true,
			json: async () => ({ results: [
				{ guestId: 'g1', status: 'failed', reason: 'load guest: timeout', retryable: true },
				{ guestId: 'g2', status: 'failed', reason: 'invalid guest field' }
			] })
		});
		const { syncQueue, getQueue } = await import('./queue');
		for (let i = 0; i < 6; i++) await syncQueue('w1');

		expect(getQueue('w1').map(m => [m.mutationId, m.attempts ?? 0])).toEqual([['m1', 0]]);
	});
});
