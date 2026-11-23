import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.fn();
const clearGuestCaches = vi.hoisted(() => vi.fn());
vi.mock('#lib/api/client.ts', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }));
vi.mock('#lib/stores/index.ts', () => ({ addToast: vi.fn() }));
vi.mock('#lib/stores/guestEvents.ts', async () => {
	const { writable } = await import('svelte/store');
	return { applyGuestResponse: vi.fn(), clearGuestCaches, guestMap: writable(new Map([['g1', { updatedAt: new Date('2025-12-31T00:00:00Z') }]])) };
});

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		get length() { return map.size; },
		key: (i: number) => [...map.keys()][i] ?? null,
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k)
	};
}

function mutation(i: number) {
	return { mutationId: `m${i}`, op: 'update' as const, guestId: 'g1', clientUpdatedAt: '2026-01-01T00:00:00Z' };
}

function ok(results: unknown[]) {
	return { ok: true, status: 200, json: async () => ({ results }) } as Response;
}

beforeEach(() => {
	vi.resetModules();
	apiFetch.mockReset();
	clearGuestCaches.mockReset();
	vi.stubGlobal('localStorage', memoryStorage());
	vi.stubGlobal('window', { dispatchEvent: () => true });
});

afterEach(() => vi.unstubAllGlobals());

describe('offline queue storage', () => {
	it('sends the queue in chunks of 100', async () => {
		const { enqueue, getQueue, syncQueue } = await import('./queue');
		for (let i = 0; i < 250; i++) enqueue('w1', mutation(i));
		apiFetch.mockImplementation(async (_p: string, init: RequestInit) =>
			ok(JSON.parse(init.body as string).mutations.map(() => ({ status: 'applied' }))));

		const r = await syncQueue('w1');

		expect(apiFetch.mock.calls.map((c) => JSON.parse(c[1].body).mutations.length)).toEqual([100, 100, 50]);
		expect(r.applied).toBe(250);
		expect(getQueue('w1')).toEqual([]);
	});

	it('serializes the read-modify-write through the Web Lock so concurrent writes do not lose a mutation', async () => {
		let chain = Promise.resolve();
		const request = vi.fn((_name: string, cb: () => void) => (chain = chain.then(() => cb())));
		vi.stubGlobal('navigator', { locks: { request } });
		const { enqueue, getQueue } = await import('./queue');

		enqueue('w1', mutation(1));
		enqueue('w1', mutation(2));
		await chain;

		expect(request).toHaveBeenCalledTimes(2);
		expect(request.mock.calls[0][0]).toBe('offline_queue_w1');
		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['m1', 'm2']);
	});

	it('stamps baseUpdatedAt from the guest this device last saw', async () => {
		const { enqueue, getQueue } = await import('./queue');
		enqueue('w1', mutation(1));
		enqueue('w1', { ...mutation(2), op: 'create', guestId: 'new' });
		expect(getQueue('w1').map((m) => m.baseUpdatedAt)).toEqual(['2025-12-31T00:00:00.000Z', null]);
	});

	it('a different user logging in drops the previous user edits', async () => {
		const { enqueue, getQueue, claimQueues } = await import('./queue');
		await claimQueues('id-a', 'alice@x.io', () => true);
		enqueue('w1', mutation(1));
		apiFetch.mockResolvedValue(ok([{ status: 'failed', reason: 'forbidden' }]));

		const ask = vi.fn(() => true);
		await claimQueues('id-b', 'bob@x.io', ask);

		expect(apiFetch).toHaveBeenCalledOnce(); // replayed first
		expect(ask).toHaveBeenCalledWith(1, 'a***@x.io');
		expect(getQueue('w1')).toEqual([]);
		expect(localStorage.getItem('offline_owner')).toBe('id-b');
		for (let i = 0; i < localStorage.length; i++) expect(localStorage.getItem(localStorage.key(i)!)).not.toContain('bob@x.io');
	});

	it('clears the previous account cached guest lists when the owner changes', async () => {
		const { claimQueues } = await import('./queue');
		await claimQueues('id-a', 'alice@x.io', () => true);
		localStorage.setItem('offline_cache_w1', '[{"email":"a@b.c"}]');

		await claimQueues('id-b', 'bob@x.io', () => true);
		expect(clearGuestCaches).toHaveBeenCalledOnce();

		clearGuestCaches.mockClear();
		await claimQueues('id-b', 'bob@x.io', () => true);
		expect(clearGuestCaches).not.toHaveBeenCalled();
	});

	it('keeps the edits when the new user declines the discard', async () => {
		const { enqueue, getQueue, claimQueues } = await import('./queue');
		await claimQueues('id-a', 'alice@x.io', () => true);
		enqueue('w1', mutation(1));
		apiFetch.mockResolvedValue(ok([{ status: 'failed' }]));

		await claimQueues('id-b', 'bob@x.io', () => false);

		expect(getQueue('w1')).toHaveLength(1);
		expect(localStorage.getItem('offline_owner')).toBe('id-a');
	});

	it('a second drain joins the one in flight instead of replaying the queue', async () => {
		const { enqueue, getQueue, syncQueue } = await import('./queue');
		enqueue('w1', mutation(1));
		let release!: (r: Response) => void;
		apiFetch.mockImplementation(() => new Promise<Response>((r) => { release = r; }));

		const first = syncQueue('w1');
		const second = syncQueue('w1');

		expect(second).toBe(first);
		expect(apiFetch).toHaveBeenCalledOnce();

		release(ok([{ status: 'applied' }]));
		expect(await first).toEqual({ applied: 1, skipped: 0, dropped: 0 });
		expect(await second).toEqual({ applied: 1, skipped: 0, dropped: 0 });
		expect(getQueue('w1')).toEqual([]);
	});

	it('drops a failed mutation only when the server marks it not retryable', async () => {
		const { enqueue, getQueue, syncQueue } = await import('./queue');
		enqueue('w1', mutation(1));
		enqueue('w1', mutation(2));
		apiFetch.mockResolvedValue(ok([
			{ status: 'failed', reason: 'angbao must not be negative', retryable: false },
			{ status: 'failed', reason: 'database is down' }
		]));

		expect(await syncQueue('w1')).toEqual({ applied: 0, skipped: 0, dropped: 1 });
		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['m2']);
	});

	it('403 keeps the queue for the owner-change prompt instead of discarding it', async () => {
		const { enqueue, getQueue, syncQueue } = await import('./queue');
		enqueue('w1', mutation(1));
		apiFetch.mockResolvedValue({ ok: false, status: 403, text: async () => 'forbidden' } as Response);

		await expect(syncQueue('w1')).rejects.toThrow(/403/);
		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['m1']);
	});

	it('merges a mutation another tab writes while enqueue is saving', async () => {
		const map = new Map<string, string>();
		let injected = false;
		vi.stubGlobal('localStorage', {
			get length() { return map.size; },
			key: (i: number) => [...map.keys()][i] ?? null,
			getItem: (k: string) => map.get(k) ?? null,
			removeItem: (k: string) => void map.delete(k),
			setItem: (k: string, v: string) => {
				map.set(k, String(v));
				if (k === 'offline_queue_w1' && !injected) {
					injected = true;
					// Another tab's write lands between our read and our next read.
					map.set(k, JSON.stringify([mutation(99)]));
				}
			}
		});
		const { enqueue, getQueue } = await import('./queue');

		enqueue('w1', mutation(1));

		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['m99', 'm1']);
	});

	it('re-drops mutations another tab writes back while dequeuing', async () => {
		const map = new Map<string, string>();
		let armed = false;
		let injected = false;
		vi.stubGlobal('localStorage', {
			get length() { return map.size; },
			key: (i: number) => [...map.keys()][i] ?? null,
			getItem: (k: string) => map.get(k) ?? null,
			removeItem: (k: string) => void map.delete(k),
			setItem: (k: string, v: string) => {
				map.set(k, String(v));
				if (armed && !injected && k === 'offline_queue_w1') {
					injected = true;
					map.set(k, JSON.stringify([mutation(1), mutation(2)]));
				}
			}
		});
		const { dequeueByIds, getQueue } = await import('./queue');
		localStorage.setItem('offline_queue_w1', JSON.stringify([mutation(1)]));
		armed = true;

		dequeueByIds('w1', ['m1']);

		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['m2']);
	});

	it('drops a chunk the server rejects outright instead of blocking the queue', async () => {
		const { enqueue, getQueue, syncQueue } = await import('./queue');
		enqueue('w1', mutation(1));
		apiFetch.mockResolvedValue({ ok: false, status: 400, text: async () => 'malformed' } as Response);

		expect(await syncQueue('w1')).toEqual({ applied: 0, skipped: 0, dropped: 0 });
		expect(getQueue('w1')).toEqual([]);
	});
});
