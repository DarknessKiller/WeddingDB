import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.fn();
vi.mock('#lib/api/client.ts', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }));
vi.mock('#lib/stores/index.ts', () => ({ addToast: vi.fn() }));
vi.mock('#lib/stores/guestEvents.ts', async () => {
	const { writable } = await import('svelte/store');
	return { applyGuestResponse: vi.fn(), guestMap: writable(new Map([['g1', { updatedAt: new Date('2025-12-31T00:00:00Z') }]])) };
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

	it('keeps the edits when the new user declines the discard', async () => {
		const { enqueue, getQueue, claimQueues } = await import('./queue');
		await claimQueues('id-a', 'alice@x.io', () => true);
		enqueue('w1', mutation(1));
		apiFetch.mockResolvedValue(ok([{ status: 'failed' }]));

		await claimQueues('id-b', 'bob@x.io', () => false);

		expect(getQueue('w1')).toHaveLength(1);
		expect(localStorage.getItem('offline_owner')).toBe('id-a');
	});
});
