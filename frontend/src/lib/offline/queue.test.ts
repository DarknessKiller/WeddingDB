import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const apiFetch = vi.hoisted(() => vi.fn());
const addToast = vi.hoisted(() => vi.fn());
vi.mock('#lib/api/client.ts', () => ({ apiFetch }));
vi.mock('#lib/stores/guestEvents.ts', () => ({ applyGuestResponse: vi.fn(), removeGuest: vi.fn() }));
vi.mock('#lib/stores/index.ts', () => ({ addToast }));

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
	addToast.mockReset();
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

	it('drops a non-retryable failure, keeps a retryable one, and tells the user both', async () => {
		const { enqueue, syncQueue, getQueue } = await import('./queue');
		enqueue('w1', mutation('bad'));
		enqueue('w1', mutation('later'));
		apiFetch.mockResolvedValue(res({ results: [
			{ guestId: 'g-bad', status: 'failed', reason: 'name too long', retryable: false },
			{ guestId: 'g-later', status: 'failed', reason: 'older than server' }
		] }));

		await syncQueue('w1');

		expect(getQueue('w1').map((m) => m.mutationId)).toEqual(['later']);
		const messages = addToast.mock.calls.map((c) => c[0]);
		expect(messages).toContain('1 offline change discarded by the server: name too long');
		expect(messages).toContain('1 offline change could not sync and will retry: older than server');
	});

	it('warns when the full queue drops its oldest change', async () => {
		const { enqueue, getQueue } = await import('./queue');
		for (let i = 0; i < 500; i++) enqueue('w1', mutation(`m${i}`));
		expect(addToast).not.toHaveBeenCalled();

		enqueue('w1', mutation('m500'));

		expect(getQueue('w1')).toHaveLength(500);
		expect(getQueue('w1')[0].mutationId).toBe('m1');
		expect(addToast).toHaveBeenCalledWith(expect.stringContaining('oldest unsynced change was dropped'), 'error');
	});
});
