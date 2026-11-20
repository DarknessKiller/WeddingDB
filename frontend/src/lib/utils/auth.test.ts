import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

function memoryStorage() {
	const map = new Map<string, string>();
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => void map.set(k, String(v)),
		removeItem: (k: string) => void map.delete(k),
		key: (i: number) => [...map.keys()][i] ?? null,
		get length() { return map.size; },
		clear: () => map.clear()
	};
}

function res(body: unknown, status = 200): Response {
	return { ok: status >= 200 && status < 300, status, json: async () => body } as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;
let location: { href: string };

beforeEach(async () => {
	vi.resetModules();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
	Object.defineProperty(globalThis, 'sessionStorage', { value: memoryStorage(), configurable: true });
	location = { href: '' };
	Object.defineProperty(globalThis, 'window', { value: { location, addEventListener: () => {}, dispatchEvent: () => {} }, configurable: true });
	fetchMock = vi.fn();
	Object.defineProperty(globalThis, 'fetch', { value: fetchMock, configurable: true });
	const { setAuth } = await import('#lib/stores/index.ts');
	setAuth('old-access', 'refresh-1', 'admin', 'A');
});

afterEach(() => {
	vi.restoreAllMocks();
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
	delete (globalThis as any).sessionStorage;
});

describe('validateToken', () => {
	it('keeps the offline queue when the server rejects the refresh token', async () => {
		localStorage.setItem('offline_queue_w1', '[{"mutationId":"m1","op":"checkin","guestId":"g1"}]');
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res({ title: 'invalid refresh token' }, 401));

		const { validateToken } = await import('./auth');
		const { getAuth } = await import('#lib/stores/index.ts');

		expect(await validateToken()).toBe(false);
		expect(getAuth().refreshToken).toBeNull();
		expect(location.href).toBe('/login');
		expect(localStorage.getItem('offline_queue_w1')).not.toBeNull();
	});

	it('keeps the session when the refresh cannot reach the server', async () => {
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockRejectedValueOnce(new TypeError('Failed to fetch'));

		const { validateToken } = await import('./auth');
		const { getAuth } = await import('#lib/stores/index.ts');

		expect(await validateToken()).toBe(true);
		expect(getAuth().refreshToken).toBe('refresh-1');
	});
});
