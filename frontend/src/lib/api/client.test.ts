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

function res(body: unknown, status = 200): Response {
	return { ok: status >= 200 && status < 300, status, json: async () => body } as Response;
}

const REFRESH_OK = { accessToken: 'new-access', role: 'admin', name: 'A' };

let fetchMock: ReturnType<typeof vi.fn>;
let location: { href: string };

beforeEach(() => {
	vi.resetModules();
	Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), configurable: true });
	localStorage.setItem('weddingdb_access_token', 'old-access');
	// Left behind by an older build; the client must evict it.
	localStorage.setItem('weddingdb_refresh_token', 'refresh-1');
	location = { href: '' };
	Object.defineProperty(globalThis, 'window', { value: { location, addEventListener: () => {} }, configurable: true });
	fetchMock = vi.fn();
	Object.defineProperty(globalThis, 'fetch', { value: fetchMock, configurable: true });
});

afterEach(() => {
	vi.restoreAllMocks();
	delete (globalThis as any).window;
	delete (globalThis as any).localStorage;
});

describe('apiFetch refresh recovery', () => {
	it('retries the request with the rotated token after a successful refresh', async () => {
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res(REFRESH_OK))
			.mockResolvedValueOnce(res({ data: [] }));

		const { apiFetch } = await import('./client');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(200);
		expect(fetchMock).toHaveBeenCalledTimes(3);
		expect(fetchMock.mock.calls[1][0]).toContain('/api/auth/refresh');
		// The refresh token is an HttpOnly cookie: the body must not carry it.
		const refreshInit = fetchMock.mock.calls[1][1] as RequestInit;
		expect(refreshInit.credentials).toBe('include');
		expect(refreshInit.body).toBe('{}');
		expect((fetchMock.mock.calls[2][1].headers as Record<string, string>).Authorization).toBe('Bearer new-access');
		expect(location.href).toBe('');
	});

	it('keeps the session when the refresh request cannot reach the server', async () => {
		fetchMock.mockResolvedValueOnce(res({}, 401)).mockRejectedValueOnce(new TypeError('Failed to fetch'));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('$lib/stores');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(401);
		expect(location.href).toBe('');
		expect(getAuth()).toMatchObject({ accessToken: 'old-access' });
		expect(localStorage.getItem('weddingdb_refresh_token')).toBe('refresh-1');
	});

	it('keeps the session when refresh fails with a server error', async () => {
		fetchMock.mockResolvedValueOnce(res({}, 401)).mockResolvedValueOnce(res({ title: 'boom' }, 502));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('$lib/stores');
		await apiFetch('/api/guests');

		expect(location.href).toBe('');
		expect(getAuth()).toMatchObject({ accessToken: 'old-access' });
	});

	it('clears the session when the server rejects the refresh token', async () => {
		fetchMock
			.mockResolvedValueOnce(res({}, 401))
			.mockResolvedValueOnce(res({ title: 'invalid refresh token' }, 401));

		const { apiFetch } = await import('./client');
		const { getAuth } = await import('$lib/stores');
		const result = await apiFetch('/api/guests');

		expect(result.status).toBe(401);
		expect(location.href).toBe('/login');
		expect(getAuth().accessToken).toBeNull();
		expect(localStorage.getItem('weddingdb_refresh_token')).toBeNull();
	});
});
