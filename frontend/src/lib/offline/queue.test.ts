import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.hoisted(() => vi.fn());
vi.mock('#lib/api/client.ts', () => ({ apiFetch }));
vi.mock('#lib/stores/guestEvents.ts', () => ({ applyGuestResponse: vi.fn(), removeGuest: vi.fn() }));
vi.mock('#lib/stores/index.ts', () => ({ addToast: vi.fn() }));

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		clear: () => map.clear()
	};
}

function res(body: unknown, status = 200): Response {
	return { ok: status >= 200 && status < 300, status, json: async () => body, text: async () => '' } as Response;
}

function mutation(id: string) {
	return { mutationId: id, op: 'update' as const, guestId: `g-${id}`, clientUpdatedAt: new Date().toISOString(), payload: { name: id } };
}

beforeEach(() => {
	vi.resetModules();
	apiFetch.mockReset();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
	Object.defineProperty(globalThis, 'window', { value: { dispatchEvent: () => true }, configurable: true });
});

afterEach(() => {
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
});

describe('syncQueue', () => {
	it('a second concurrent call joins the running sync instead of resending', async () => {
		const { enqueue, syncQueue, getQueue } = await import('./queue');
		enqueue('w1', mutation('m1'));
		let release!: (r: Response) => void;
		apiFetch.mockReturnValueOnce(new Promise<Response>((r) => { release = r; }));

		const first = syncQueue('w1');
		const second = syncQueue('w1');
		release(res({ results: [{ guestId: 'g-m1', status: 'applied' }] }));

		expect(await first).toEqual(await second);
		expect(apiFetch).toHaveBeenCalledTimes(1);
		expect(getQueue('w1')).toEqual([]);
	});

	it('sends each queued mutationId unchanged on replay', async () => {
		const { enqueue, syncQueue } = await import('./queue');
		enqueue('w1', mutation('m1'));
		apiFetch.mockResolvedValue(res({ results: [{ guestId: 'g-m1', status: 'failed', reason: 'busy' }] }));

		await syncQueue('w1');
		await syncQueue('w1');

		const ids = apiFetch.mock.calls.map((c) => JSON.parse(c[1].body).mutations[0].mutationId);
		expect(ids).toEqual(['m1', 'm1']);
	});
});
